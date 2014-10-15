package router

import (
	"code.google.com/p/gopacket"
	"code.google.com/p/gopacket/layers"
	"log"
	"net"
	"syscall"
	"time"
)

func (conn *LocalConnection) ensureForwarders() error {
	if conn.forwardChan != nil || conn.forwardChanDF != nil {
		return nil
	}
	udpSender := NewSimpleUDPSender(conn)
	udpSenderDF, err := NewRawUDPSender(conn) // only thing that can error, so do it early
	if err != nil {
		return err
	}

	usingPassword := conn.SessionKey != nil
	var encryptor, encryptorDF Encryptor
	if usingPassword {
		encryptor = NewNaClEncryptor(conn.local.NameByte, conn, false)
		encryptorDF = NewNaClEncryptor(conn.local.NameByte, conn, true)
	} else {
		encryptor = NewNonEncryptor(conn.local.NameByte)
		encryptorDF = NewNonEncryptor(conn.local.NameByte)
	}

	var (
		forwardChan   = make(chan *ForwardedFrame, ChannelSize)
		forwardChanDF = make(chan *ForwardedFrame, ChannelSize)
		stopForward   = make(chan interface{}, 0)
		stopForwardDF = make(chan interface{}, 0)
		verifyPMTU    = make(chan int, ChannelSize)
	)
	//NB: only forwarderDF can ever encounter EMSGSIZE errors, and
	//thus perform PMTU verification
	forwarder := NewForwarder(conn, forwardChan, stopForward, nil, encryptor, udpSender, DefaultPMTU)
	forwarderDF := NewForwarder(conn, forwardChanDF, stopForwardDF, verifyPMTU, encryptorDF, udpSenderDF, DefaultPMTU)

	// Various fields in the conn struct are read by other processes,
	// so we have to use locks.
	conn.Lock()
	conn.forwardChan = forwardChan
	conn.forwardChanDF = forwardChanDF
	conn.stopForward = stopForward
	conn.stopForwardDF = stopForwardDF
	conn.verifyPMTU = verifyPMTU
	conn.effectivePMTU = forwarder.unverifiedPMTU
	conn.Unlock()

	go forwarder.Run()
	go forwarderDF.Run()

	return nil
}

func (conn *LocalConnection) stopForwarders() {
	conn.Lock()
	conn.forwardChan = nil
	conn.forwardChanDF = nil
	conn.Unlock()
	// Now signal the forwarder loops to exit. They will drain the
	// forwarder chans in order to unblock any router processes
	// blocked on sending.
	if conn.stopForward != nil {
		conn.stopForward <- nil
		conn.stopForwardDF <- nil
	}
}

// Called from peer.Relay[Broadcast] which is itself invoked from
// router (both UDP listener process and sniffer process). Also called
// from connection's heartbeat process, and from the connection's TCP
// receiver process.
func (conn *LocalConnection) Forward(df bool, frame *ForwardedFrame, dec *EthernetDecoder) error {
	conn.RLock()
	var (
		forwardChan   = conn.forwardChan
		forwardChanDF = conn.forwardChanDF
		effectivePMTU = conn.effectivePMTU
		stackFrag     = conn.stackFrag
	)
	conn.RUnlock()

	if forwardChan == nil || forwardChanDF == nil {
		conn.log("Cannot forward frame yet - awaiting contact")
		return nil
	}
	if df {
		if !frameTooBig(frame, effectivePMTU) {
			forwardChanDF <- frame
			return nil
		}
		return FrameTooBigError{EPMTU: effectivePMTU}
	} else {
		if stackFrag || dec == nil || len(dec.decoded) < 2 {
			forwardChan <- frame
			return nil
		}
		// Don't have trustworthy stack, so we're going to have to
		// send it DF in any case.
		if !frameTooBig(frame, effectivePMTU) {
			forwardChanDF <- frame
			return nil
		}
		conn.Router.LogFrame("Fragmenting", frame.frame, &dec.eth)
		// We can't trust the stack to fragment, we have IP, and we
		// have a frame that's too big for the MTU, so we have to
		// fragment it ourself.
		return fragment(dec.eth, dec.ip, effectivePMTU, frame, func(segFrame *ForwardedFrame) {
			forwardChanDF <- segFrame
		})
	}
}

func frameTooBig(frame *ForwardedFrame, effectivePMTU int) bool {
	// We capture/forward complete ethernet frames. Therefore the
	// frame length includes the ethernet header. However, MTUs
	// operate at the IP layer and thus do not include the ethernet
	// header. To put it another way, when a sender that was told an
	// MTU of M sends an IP packet of exactly that length, we will
	// capture/forward M + EthernetOverhead bytes of data.
	return len(frame.frame) > effectivePMTU+EthernetOverhead
}

func fragment(eth layers.Ethernet, ip layers.IPv4, pmtu int, frame *ForwardedFrame, forward func(*ForwardedFrame)) error {
	// We are not doing any sort of NAT, so we don't need to worry
	// about checksums of IP payload (eg UDP checksum).
	headerSize := int(ip.IHL) * 4
	// &^ is bit clear (AND NOT). So here we're clearing the lowest 3
	// bits.
	maxSegmentSize := (pmtu - headerSize) &^ 7
	opts := gopacket.SerializeOptions{
		FixLengths:       false,
		ComputeChecksums: true}
	payloadSize := int(ip.Length) - headerSize
	payload := ip.BaseLayer.Payload[:payloadSize]
	offsetBase := int(ip.FragOffset) << 3
	origFlags := ip.Flags
	ip.Flags = ip.Flags | layers.IPv4MoreFragments
	ip.Length = uint16(headerSize + maxSegmentSize)
	if eth.EthernetType == layers.EthernetTypeLLC {
		// using LLC, so must set eth length correctly. eth length
		// is just the length of the payload
		eth.Length = ip.Length
	} else {
		eth.Length = 0
	}
	for offset := 0; offset < payloadSize; offset += maxSegmentSize {
		var segmentPayload []byte
		if len(payload) <= maxSegmentSize {
			// last one
			segmentPayload = payload
			ip.Length = uint16(len(payload) + headerSize)
			ip.Flags = origFlags
			if eth.EthernetType == layers.EthernetTypeLLC {
				eth.Length = ip.Length
			} else {
				eth.Length = 0
			}
		} else {
			segmentPayload = payload[:maxSegmentSize]
			payload = payload[maxSegmentSize:]
		}
		ip.FragOffset = uint16((offset + offsetBase) >> 3)
		buf := gopacket.NewSerializeBuffer()
		segPayload := gopacket.Payload(segmentPayload)
		err := gopacket.SerializeLayers(buf, opts, &eth, &ip, &segPayload)
		if err != nil {
			return err
		}
		// make copies of the frame we received
		var segFrame ForwardedFrame = *frame
		segFrame.frame = buf.Bytes()
		forward(&segFrame)
	}
	return nil
}

// Forwarder

func NewForwarder(conn *LocalConnection, ch <-chan *ForwardedFrame, stop <-chan interface{}, verifyPMTU <-chan int, enc Encryptor, udpSender UDPSender, pmtu int) *Forwarder {
	fwd := &Forwarder{
		conn:       conn,
		ch:         ch,
		stop:       stop,
		verifyPMTU: verifyPMTU,
		enc:        enc,
		udpSender:  udpSender}
	fwd.unverifiedPMTU = pmtu - fwd.effectiveOverhead()
	fwd.maxPayload = pmtu - UDPOverhead
	return fwd
}

func (fwd *Forwarder) Run() {
	defer fwd.udpSender.Shutdown()
	var flushed, ok bool
	var frame *ForwardedFrame
	for {
		flushed = false
		select {
		case <-fwd.stop:
			fwd.drain()
			return
		case <-fwd.verifyPMTUTick:
			// We only do this case here when we know the buffers are
			// all empty so that we don't risk appending verify-frames
			// to other data.
			fwd.verifyPMTUTick = nil
			if fwd.pmtuVerified {
				continue
			}
			if fwd.pmtuVerifyCount > 0 {
				fwd.pmtuVerifyCount--
				fwd.attemptVerifyEffectivePMTU()
			} else {
				// we've exceeded the verification attempts of the
				// unverifiedPMTU
				fwd.lowestBadPMTU = fwd.unverifiedPMTU
				fwd.verifyEffectivePMTU((fwd.highestGoodPMTU + fwd.lowestBadPMTU) / 2)
			}
		case epmtu := <-fwd.verifyPMTU:
			if fwd.pmtuVerified || epmtu != fwd.unverifiedPMTU {
				continue
			}
			if epmtu+1 < fwd.lowestBadPMTU {
				fwd.highestGoodPMTU = fwd.unverifiedPMTU // = epmtu
				fwd.verifyEffectivePMTU((fwd.highestGoodPMTU + fwd.lowestBadPMTU) / 2)
			} else {
				fwd.pmtuVerified = true
				fwd.maxPayload = epmtu + fwd.effectiveOverhead() - UDPOverhead
				fwd.conn.setEffectivePMTU(epmtu)
				fwd.conn.log("Effective PMTU verified at", epmtu)
			}
		case frame = <-fwd.ch:
			if !fwd.appendFrame(frame) {
				fwd.logDrop(frame)
				continue
			}
			for !flushed {
				select {
				case frame, ok = <-fwd.ch:
					if !ok {
						return
					}
					if !fwd.appendFrame(frame) {
						fwd.flush()
						if !fwd.appendFrame(frame) {
							fwd.logDrop(frame)
							flushed = true
						}
					}
				default:
					fwd.flush()
					flushed = true
				}
			}
		}
	}
}

func (fwd *Forwarder) effectiveOverhead() int {
	return UDPOverhead + fwd.enc.PacketOverhead() + fwd.enc.FrameOverhead() + EthernetOverhead
}

func (fwd *Forwarder) verifyEffectivePMTU(newUnverifiedPMTU int) {
	fwd.unverifiedPMTU = newUnverifiedPMTU
	fwd.pmtuVerifyCount = PMTUVerifyAttempts
	fwd.attemptVerifyEffectivePMTU()
}

func (fwd *Forwarder) attemptVerifyEffectivePMTU() {
	pmtuVerifyFrame := &ForwardedFrame{
		srcPeer: fwd.conn.local,
		dstPeer: fwd.conn.remote,
		frame:   make([]byte, fwd.unverifiedPMTU+EthernetOverhead)}
	fwd.enc.AppendFrame(pmtuVerifyFrame)
	fwd.flush()
	if fwd.verifyPMTUTick == nil {
		fwd.verifyPMTUTick = time.After(PMTUVerifyTimeout << (PMTUVerifyAttempts - fwd.pmtuVerifyCount))
	}
}

func (fwd *Forwarder) appendFrame(frame *ForwardedFrame) bool {
	frameLen := len(frame.frame)
	if fwd.enc.TotalLen()+fwd.enc.FrameOverhead()+frameLen > fwd.maxPayload {
		return false
	}
	fwd.enc.AppendFrame(frame)
	return true
}

func (fwd *Forwarder) flush() {
	err := fwd.udpSender.Send(fwd.enc.Bytes())
	if err != nil {
		if mtbe, ok := err.(MsgTooBigError); ok {
			newUnverifiedPMTU := mtbe.PMTU - fwd.effectiveOverhead()
			if newUnverifiedPMTU >= fwd.unverifiedPMTU {
				return
			}
			fwd.pmtuVerified = false
			fwd.maxPayload = mtbe.PMTU - UDPOverhead
			fwd.highestGoodPMTU = 8
			fwd.lowestBadPMTU = newUnverifiedPMTU + 1
			fwd.conn.setEffectivePMTU(newUnverifiedPMTU)
			fwd.verifyEffectivePMTU(newUnverifiedPMTU)
		} else if PosixError(err) == syscall.ENOBUFS {
			// TODO handle this better
		} else {
			fwd.conn.CheckFatal(err)
		}
	}
}

func (fwd *Forwarder) drain() {
	// We want to drain before exiting otherwise we could get the
	// packet sniffer or udp listener blocked on sending to a full
	// chan
	for {
		select {
		case <-fwd.ch:
		default:
			return
		}
	}
}

func (fwd *Forwarder) logDrop(frame *ForwardedFrame) {
	fwd.conn.log("Dropping too big frame during forwarding: frame len:", len(frame.frame), "; effective PMTU:", fwd.maxPayload+UDPOverhead-fwd.effectiveOverhead())
}

// UDP Senders

func NewSimpleUDPSender(conn *LocalConnection) *SimpleUDPSender {
	return &SimpleUDPSender{udpConn: conn.Router.UDPListener, conn: conn}
}

func (sender *SimpleUDPSender) Send(msg []byte) error {
	_, err := sender.udpConn.WriteToUDP(msg, sender.conn.RemoteUDPAddr())
	return err
}

func (sender *SimpleUDPSender) Shutdown() error {
	return nil
}

func NewRawUDPSender(conn *LocalConnection) (*RawUDPSender, error) {
	ipSocket, err := dialIP(conn)
	if err != nil {
		return nil, err
	}
	udpHeader := &layers.UDP{SrcPort: layers.UDPPort(Port)}
	ipBuf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		FixLengths: true,
		// UDP header is calculated with a phantom IP
		// header. Yes, it's totally nuts. Thankfully, for UDP
		// over IPv4, the checksum is optional. It's not
		// optional for IPv6, but we'll ignore that for
		// now. TODO
		ComputeChecksums: false}

	return &RawUDPSender{
		ipBuf:     ipBuf,
		opts:      opts,
		udpHeader: udpHeader,
		socket:    ipSocket,
		conn:      conn}, nil
}

func (sender *RawUDPSender) Send(msg []byte) error {
	payload := gopacket.Payload(msg)
	sender.udpHeader.DstPort = layers.UDPPort(sender.conn.RemoteUDPAddr().Port)

	err := gopacket.SerializeLayers(sender.ipBuf, sender.opts, sender.udpHeader, &payload)
	if err != nil {
		return err
	}
	packet := sender.ipBuf.Bytes()
	_, err = sender.socket.Write(packet)
	if err == nil || PosixError(err) != syscall.EMSGSIZE {
		return err
	}
	f, err := sender.socket.File()
	if err != nil {
		return err
	}
	defer f.Close()
	fd := int(f.Fd())
	log.Println("EMSGSIZE on send, expecting PMTU update (IP packet was",
		len(packet), "bytes, payload was", len(msg), "bytes)")
	pmtu, err := syscall.GetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_MTU)
	if err != nil {
		return err
	}
	return MsgTooBigError{PMTU: pmtu}
}

func (sender *RawUDPSender) Shutdown() error {
	defer func() { sender.socket = nil }()
	return sender.socket.Close()
}

func dialIP(conn *LocalConnection) (*net.IPConn, error) {
	ipLocalAddr, err := ipAddr(conn.TCPConn.LocalAddr())
	if err != nil {
		return nil, err
	}
	ipRemoteAddr, err := ipAddr(conn.TCPConn.RemoteAddr())
	if err != nil {
		return nil, err
	}
	ipSocket, err := net.DialIP("ip4:UDP", ipLocalAddr, ipRemoteAddr)
	if err != nil {
		return nil, err
	}
	f, err := ipSocket.File()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fd := int(f.Fd())
	// This Makes sure all packets we send out have DF set on them.
	err = syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_MTU_DISCOVER, syscall.IP_PMTUDISC_DO)
	if err != nil {
		return nil, err
	}
	return ipSocket, nil
}

func ipAddr(addr net.Addr) (*net.IPAddr, error) {
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return nil, err
	}
	return &net.IPAddr{
		IP:   net.ParseIP(host),
		Zone: ""}, nil
}
