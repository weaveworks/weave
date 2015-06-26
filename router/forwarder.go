package router

import (
	"syscall"
	"time"

	"code.google.com/p/gopacket"
	"code.google.com/p/gopacket/layers"
)

type ForwardedFrame struct {
	srcPeer *Peer
	dstPeer *Peer
	frame   []byte
}

type FrameTooBigError struct {
	EPMTU int // effective pmtu, i.e. what we tell packet senders
}

func (conn *LocalConnection) ensureForwarders() error {
	if conn.forwarder != nil || conn.forwarderDF != nil {
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
		encryptor = NewNaClEncryptor(conn.local.NameByte, conn.SessionKey, conn.outbound, false)
		encryptorDF = NewNaClEncryptor(conn.local.NameByte, conn.SessionKey, conn.outbound, true)
	} else {
		encryptor = NewNonEncryptor(conn.local.NameByte)
		encryptorDF = NewNonEncryptor(conn.local.NameByte)
	}

	forwarder := NewForwarder(conn, encryptor, udpSender, DefaultPMTU)
	forwarderDF := NewForwarderDF(conn, encryptorDF, udpSenderDF, DefaultPMTU)

	// Various fields in the conn struct are read by other processes,
	// so we have to use locks.
	conn.Lock()
	conn.forwarder = forwarder
	conn.forwarderDF = forwarderDF
	conn.Unlock()

	return nil
}

func (conn *LocalConnection) stopForwarders() {
	if conn.forwarder == nil || conn.forwarderDF == nil {
		return
	}
	conn.forwarder.Shutdown()
	conn.forwarderDF.Shutdown()
}

// Called from connection's actor process, and from the connection's
// TCP receiver process.
func (conn *LocalConnection) Send(df bool, frameBytes []byte) error {
	frame := &ForwardedFrame{
		srcPeer: conn.local,
		dstPeer: conn.remote,
		frame:   frameBytes}
	return conn.forward(frame, nil, df)
}

// Called from LocalPeer.Relay[Broadcast] which is itself invoked from
// router (both UDP listener process and sniffer process).
func (conn *LocalConnection) Forward(frame *ForwardedFrame, dec *EthernetDecoder) error {
	return conn.forward(frame, dec, dec != nil && dec.DF())
}

func (conn *LocalConnection) forward(frame *ForwardedFrame, dec *EthernetDecoder, df bool) error {
	conn.RLock()
	var (
		forwarder     = conn.forwarder
		forwarderDF   = conn.forwarderDF
		effectivePMTU = conn.effectivePMTU
		stackFrag     = conn.stackFrag
	)
	conn.RUnlock()

	if forwarder == nil || forwarderDF == nil {
		conn.Log("Cannot forward frame yet - awaiting contact")
		return nil
	}

	// We could use non-blocking channel sends here, i.e. drop frames
	// on the floor when the forwarder is busy. This would allow our
	// caller - the capturing loop in the router - to read frames more
	// quickly when under load, i.e. we'd drop fewer frames on the
	// floor during capture. And we could maximise CPU utilisation
	// since we aren't stalling a thread. However, a lot of work has
	// already been done by the time we get here. Since any packet we
	// drop will likely get re-transmitted we end up paying that cost
	// multiple times. So it's better to drop things at the beginning
	// of our pipeline.
	if df {
		if !frameTooBig(frame, effectivePMTU) {
			forwarderDF.Forward(frame)
			return nil
		}
		return FrameTooBigError{EPMTU: effectivePMTU}
	}

	if stackFrag || dec == nil || len(dec.decoded) < 2 {
		forwarder.Forward(frame)
		return nil
	}
	// Don't have trustworthy stack, so we're going to have to
	// send it DF in any case.
	if !frameTooBig(frame, effectivePMTU) {
		forwarderDF.Forward(frame)
		return nil
	}
	conn.Router.LogFrame("Fragmenting", frame.frame, dec)
	// We can't trust the stack to fragment, we have IP, and we
	// have a frame that's too big for the MTU, so we have to
	// fragment it ourself.
	return fragment(dec.Eth, dec.IP, effectivePMTU, frame, func(segFrame *ForwardedFrame) {
		forwarderDF.Forward(segFrame)
	})
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
		segFrame := *frame
		segFrame.frame = buf.Bytes()
		forward(&segFrame)
	}
	return nil
}

// Forwarder

type Forwarder struct {
	conn             *LocalConnection
	ch               chan<- *ForwardedFrame
	finished         <-chan struct{}
	enc              Encryptor
	udpSender        UDPSender
	maxPayload       int
	processSendError func(error) error
}

func NewForwarder(conn *LocalConnection, enc Encryptor, udpSender UDPSender, pmtu int) *Forwarder {
	ch := make(chan *ForwardedFrame, ChannelSize)
	finished := make(chan struct{})
	fwd := &Forwarder{
		conn:             conn,
		ch:               ch,
		finished:         finished,
		enc:              enc,
		udpSender:        udpSender,
		maxPayload:       pmtu - UDPOverhead,
		processSendError: func(err error) error { return err }}
	go fwd.run(ch, finished)
	return fwd
}

func (fwd *Forwarder) Shutdown() {
	fwd.ch <- nil
}

func (fwd *Forwarder) Forward(frame *ForwardedFrame) {
	select {
	case fwd.ch <- frame:
	case <-fwd.finished:
	}
}

func (fwd *Forwarder) run(ch <-chan *ForwardedFrame, finished chan<- struct{}) {
	defer fwd.udpSender.Shutdown()
	for {
		if !fwd.accumulateAndSendFrames(ch, <-ch) {
			close(finished)
			return
		}
	}
}

func (fwd *Forwarder) effectiveOverhead() int {
	return UDPOverhead + fwd.enc.PacketOverhead() + fwd.enc.FrameOverhead() + EthernetOverhead
}

// Drain the inbound channel of frames, aggregating them into larger
// packets for efficient transmission.
//
// FIXME Depending on the golang scheduler, and the rate at which
// franes get sent to the forwarder, we can be going around this loop
// forever. That is bad since there may be other stuff for us to do,
// i.e. the other branches in the run loop.
func (fwd *Forwarder) accumulateAndSendFrames(ch <-chan *ForwardedFrame, frame *ForwardedFrame) bool {
	if frame == nil {
		return false
	}
	if !fwd.appendFrame(frame) {
		fwd.logDrop(frame)
		// [1] The buffer is empty at this point and therefore we must
		// not flush it. The easiest way to accomplish that is simply
		// by returning to the surrounding run loop.
		return true
	}
	for {
		select {
		case frame = <-ch:
			if frame == nil {
				return false
			}
			if !fwd.appendFrame(frame) {
				fwd.flush()
				if !fwd.appendFrame(frame) {
					fwd.logDrop(frame)
					return true // see [1]
				}
			}
		default:
			fwd.flush()
			return true
		}
	}
}

func (fwd *Forwarder) logDrop(frame *ForwardedFrame) {
	fwd.conn.Log("Dropping too big frame during forwarding: frame len:", len(frame.frame), "; effective PMTU:", fwd.maxPayload+UDPOverhead-fwd.effectiveOverhead())
}

func (fwd *Forwarder) appendFrame(frame *ForwardedFrame) bool {
	frameLen := len(frame.frame)
	if fwd.enc.TotalLen()+fwd.enc.FrameOverhead()+frameLen > fwd.maxPayload {
		return false
	}
	fwd.enc.AppendFrame(frame.srcPeer.NameByte, frame.dstPeer.NameByte, frame.frame)
	return true
}

func (fwd *Forwarder) flush() {
	msg, err := fwd.enc.Bytes()
	if err != nil {
		fwd.conn.Shutdown(err)
	}
	err = fwd.processSendError(fwd.udpSender.Send(msg))
	if err != nil && PosixError(err) != syscall.ENOBUFS {
		fwd.conn.Shutdown(err)
	}
}

type ForwarderDF struct {
	Forwarder
	verifyPMTUTick  <-chan time.Time
	verifyPMTU      chan<- int
	pmtuVerifyCount uint
	pmtuVerified    bool
	highestGoodPMTU int
	unverifiedPMTU  int
	lowestBadPMTU   int
}

func NewForwarderDF(conn *LocalConnection, enc Encryptor, udpSender UDPSender, pmtu int) *ForwarderDF {
	ch := make(chan *ForwardedFrame, ChannelSize)
	finished := make(chan struct{})
	verifyPMTU := make(chan int, ChannelSize)
	fwd := &ForwarderDF{
		Forwarder: Forwarder{
			conn:       conn,
			ch:         ch,
			finished:   finished,
			enc:        enc,
			udpSender:  udpSender,
			maxPayload: pmtu - UDPOverhead},
		verifyPMTU: verifyPMTU}
	fwd.Forwarder.processSendError = fwd.processSendError
	fwd.unverifiedPMTU = pmtu - fwd.effectiveOverhead()
	conn.setEffectivePMTU(fwd.unverifiedPMTU)
	go fwd.run(ch, finished, verifyPMTU)
	return fwd
}

func (fwd *ForwarderDF) PMTUVerified(pmtu int) {
	select {
	case fwd.verifyPMTU <- pmtu:
	case <-fwd.finished:
	}
}

func (fwd *ForwarderDF) run(ch <-chan *ForwardedFrame, finished chan<- struct{}, verifyPMTU <-chan int) {
	defer fwd.udpSender.Shutdown()
	for {
		select {
		case <-fwd.verifyPMTUTick:
			// We only do this case here when we know the
			// buffers are all empty so that we don't risk
			// appending verify-frames to other data.
			fwd.verifyPMTUTick = nil
			if fwd.pmtuVerified {
				continue
			}
			if fwd.pmtuVerifyCount > 0 {
				fwd.pmtuVerifyCount--
				fwd.attemptVerifyEffectivePMTU()
			} else {
				// we've exceeded the verification
				// attempts of the unverifiedPMTU
				fwd.lowestBadPMTU = fwd.unverifiedPMTU
				fwd.verifyEffectivePMTU((fwd.highestGoodPMTU + fwd.lowestBadPMTU) / 2)
			}
		case epmtu := <-verifyPMTU:
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
				fwd.conn.Log("Effective PMTU verified at", epmtu)
			}
		case frame := <-ch:
			if !fwd.accumulateAndSendFrames(ch, frame) {
				close(finished)
				return
			}
		}
	}
}

func (fwd *ForwarderDF) verifyEffectivePMTU(newUnverifiedPMTU int) {
	fwd.unverifiedPMTU = newUnverifiedPMTU
	fwd.pmtuVerifyCount = PMTUVerifyAttempts
	fwd.attemptVerifyEffectivePMTU()
}

func (fwd *ForwarderDF) attemptVerifyEffectivePMTU() {
	fwd.enc.AppendFrame(fwd.conn.local.NameByte, fwd.conn.remote.NameByte,
		make([]byte, fwd.unverifiedPMTU+EthernetOverhead))
	fwd.flush()
	if fwd.verifyPMTUTick == nil {
		fwd.verifyPMTUTick = time.After(PMTUVerifyTimeout << (PMTUVerifyAttempts - fwd.pmtuVerifyCount))
	}
}

func (fwd *ForwarderDF) processSendError(err error) error {
	if mtbe, ok := err.(MsgTooBigError); ok {
		newUnverifiedPMTU := mtbe.PMTU - fwd.effectiveOverhead()
		if newUnverifiedPMTU < fwd.unverifiedPMTU {
			fwd.pmtuVerified = false
			fwd.maxPayload = mtbe.PMTU - UDPOverhead
			fwd.highestGoodPMTU = 8
			fwd.lowestBadPMTU = newUnverifiedPMTU + 1
			fwd.conn.setEffectivePMTU(newUnverifiedPMTU)
			fwd.verifyEffectivePMTU(newUnverifiedPMTU)
		}

		return nil
	}

	return err
}
