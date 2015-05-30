// This contains the InterHost implementation for weave's own UDP
// encapsulation protocol ("sleeve" because a sleeve encapsulates
// something, it's often woven, it rhymes with "weave", make up your
// own cheesy reason).

package router

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"syscall"
	"time"

	"code.google.com/p/gopacket"
	"code.google.com/p/gopacket/layers"
)

type SleeveInterHost struct {
	localPort int

	// These fields are set in ConsumePackets, and not
	// subsequently modified
	localPeer    *Peer
	localPeerBin []byte
	consumer     InterHostConsumer
	peers        *Peers
	conn         *net.UDPConn

	lock       sync.Mutex
	forwarders map[PeerName]*sleeveForwarder
}

func NewSleeveInterHost(localPort int) InterHost {
	return &SleeveInterHost{localPort: localPort}
}

func (sleeve *SleeveInterHost) ConsumeInterHostPackets(localPeer *Peer,
	peers *Peers, consumer InterHostConsumer) error {
	localAddr, err := net.ResolveUDPAddr("udp4",
		fmt.Sprint(":", sleeve.localPort))
	if err != nil {
		return err
	}

	conn, err := net.ListenUDP("udp4", localAddr)
	if err != nil {
		return err
	}

	f, err := conn.File()
	if err != nil {
		return err
	}

	defer f.Close()
	fd := int(f.Fd())

	// This makes sure all packets we send out do not have DF set
	// on them.
	err = syscall.SetsockoptInt(fd, syscall.IPPROTO_IP,
		syscall.IP_MTU_DISCOVER, syscall.IP_PMTUDISC_DONT)
	if err != nil {
		return err
	}

	sleeve.lock.Lock()
	defer sleeve.lock.Unlock()

	if sleeve.localPeer != nil {
		conn.Close()
		return fmt.Errorf("ConsumePackets already called")
	}

	sleeve.localPeer = localPeer
	sleeve.localPeerBin = localPeer.NameByte
	sleeve.consumer = consumer
	sleeve.peers = peers
	sleeve.conn = conn
	sleeve.forwarders = make(map[PeerName]*sleeveForwarder)
	go sleeve.readUDP()
	return nil
}

func (*SleeveInterHost) InvalidateRoutes() {
	// no cached information, so nothing to do
}

func (*SleeveInterHost) InvalidateShortIDs() {
	// no cached information, so nothing to do
}

func (sleeve *SleeveInterHost) logFrame(string, []byte, *layers.Ethernet) {
	// XXX
}

func (sleeve *SleeveInterHost) lookupForwarder(peer PeerName) *sleeveForwarder {
	sleeve.lock.Lock()
	defer sleeve.lock.Unlock()
	return sleeve.forwarders[peer]
}

func (sleeve *SleeveInterHost) readUDP() {
	defer sleeve.conn.Close()
	dec := NewEthernetDecoder()
	buf := make([]byte, MaxUDPPacketSize)

	for {
		n, sender, err := sleeve.conn.ReadFromUDP(buf)
		if err == io.EOF {
			return
		} else if err != nil {
			log.Println("ignoring UDP read error", err)
			continue
		} else if n < NameSize {
			log.Println("ignoring too short UDP packet from", sender)
			continue
		}

		fwdName := PeerNameFromBin(buf[:NameSize])
		fwd := sleeve.lookupForwarder(fwdName)
		if fwd == nil {
			continue
		}

		packet := make([]byte, n-NameSize)
		copy(packet, buf[NameSize:n])

		err = fwd.crypto.Dec.IterateFrames(packet,
			func(src []byte, dst []byte, frame []byte) {
				sleeve.handleFrame(sender, fwd,
					src, dst, frame, dec)
			})
		if err != nil {
			// Errors during UDP packet decoding /
			// processing are non-fatal. One common cause
			// is that we receive and attempt to decrypt a
			// "stray" packet. This can actually happen
			// quite easily if there is some connection
			// churn between two peers. After all, UDP
			// isn't a connection-oriented protocol, yet
			// we pretend it is.
			//
			// If anything really is seriously,
			// unrecoverably amiss with a connection, that
			// will typically result in missed heartbeats
			// and the connection getting shut down
			// because of that.
			fwd.logFrom(sender, err)
		}
	}
}

func (sleeve *SleeveInterHost) handleFrame(sender *net.UDPAddr,
	fwd *sleeveForwarder, src []byte, dst []byte, frame []byte,
	dec *EthernetDecoder) {
	dec.DecodeLayers(frame)
	decodedLen := len(dec.decoded)
	if decodedLen == 0 {
		return
	}

	srcPeer, found := sleeve.peers.Fetch(PeerNameFromBin(src))
	if !found {
		return
	}

	dstPeer, found := sleeve.peers.Fetch(PeerNameFromBin(dst))
	if !found {
		return
	}

	// Handle special frames produced internally (rather than
	// captured/forwarded) by the remote router.
	//
	// We really shouldn't be decoding these above, since they are
	// not genuine Ethernet frames. However, it is actually more
	// efficient to do so, as we want to optimise for the common
	// (i.e. non-special) frames. These always need decoding, and
	// detecting special frames is cheaper post decoding than pre.
	if decodedLen == 1 && dec.IsSpecial() {
		if srcPeer == fwd.remotePeer && dstPeer == fwd.sleeve.localPeer {
			select {
			case fwd.specialChan <- specialFrame{sender, frame}:
			case <-fwd.finishedChan:
			}
		}

		return
	}

	sleeve.sendToConsumer(srcPeer, dstPeer, frame, dec)
}

func (sleeve *SleeveInterHost) sendToConsumer(srcPeer, dstPeer *Peer,
	frame []byte, dec *EthernetDecoder) {
	if sleeve.consumer == nil {
		return
	}

	fop := sleeve.consumer(ForwardPacketKey{
		SrcPeer:   srcPeer,
		DstPeer:   dstPeer,
		PacketKey: dec.PacketKey(),
	})
	if fop != nil {
		fop.Send(frame, dec, false)
	}
}

type udpSender interface {
	send([]byte, *net.UDPAddr) error
}

func (sleeve *SleeveInterHost) send(msg []byte, raddr *net.UDPAddr) error {
	sleeve.lock.Lock()
	conn := sleeve.conn
	sleeve.lock.Unlock()

	if conn == nil {
		// Consume wasn't called yet
		return nil
	}

	_, err := conn.WriteToUDP(msg, raddr)
	return err
}

type sleeveForwarder struct {
	// Immutable
	sleeve         *SleeveInterHost
	remotePeer     *Peer
	remotePeerBin  []byte
	sendControlMsg func([]byte) error
	uid            uint64

	// Channels to communicate with the aggregator goroutine
	aggregatorChan   chan<- aggregatorFrame
	aggregatorDFChan chan<- aggregatorFrame
	specialChan      chan<- specialFrame
	confirmedChan    chan<- struct{}
	finishedChan     <-chan struct{}

	// Explicitly locked state
	lock       sync.RWMutex
	listener   InterHostForwarderListener
	remoteAddr *net.UDPAddr

	// These fields are accessed and updated independently, so no
	// locking needed
	effectivePMTU int
	stackFrag     bool

	// dec is only used from the readUDP goroutine, enc and encDF
	// are only used in the forwarder goroutine
	crypto InterHostCrypto

	// State only used within the forwarder goroutine
	senderDF          *udpSenderDF
	maxPayload        int
	heartbeatInterval time.Duration
	heartbeatTimer    *time.Timer
	heartbeatTimeout  *time.Timer
	fragTestTicker    *time.Ticker
	ackedHeartbeat    bool

	pmtuTestTimeout  *time.Timer
	pmtuTestsSent    uint
	epmtuHighestGood int
	epmtuLowestBad   int
	epmtuCandidate   int
}

type aggregatorFrame struct {
	src   []byte
	dst   []byte
	frame []byte
}

// A "special" message over UDP, or a control message.  The sender is
// nil for control messages.
type specialFrame struct {
	sender *net.UDPAddr
	frame  []byte
}

func (sleeve *SleeveInterHost) MakeForwarder(remotePeer *Peer,
	localIP net.IP, remote *net.UDPAddr,
	connUid uint64, crypto InterHostCrypto,
	sendControlMsg func([]byte) error) (InterHostForwarder, error) {
	aggChan := make(chan aggregatorFrame, ChannelSize)
	aggDFChan := make(chan aggregatorFrame, ChannelSize)
	specialChan := make(chan specialFrame, 1)
	confirmedChan := make(chan struct{})
	finishedChan := make(chan struct{})
	remotePeerBin := remotePeer.NameByte

	fwd := &sleeveForwarder{
		sleeve:           sleeve,
		remotePeer:       remotePeer,
		remotePeerBin:    remotePeerBin,
		sendControlMsg:   sendControlMsg,
		uid:              connUid,
		aggregatorChan:   aggChan,
		aggregatorDFChan: aggDFChan,
		specialChan:      specialChan,
		confirmedChan:    confirmedChan,
		finishedChan:     finishedChan,
		remoteAddr:       remote,
		effectivePMTU:    DefaultPMTU,
		crypto:           crypto,
		senderDF:         newUDPSenderDF(localIP, sleeve.localPort),
		maxPayload:       DefaultPMTU - UDPOverhead,
	}

	// XXX handle case where remotePeer already registered
	sleeve.lock.Lock()
	sleeve.forwarders[remotePeer.Name] = fwd
	sleeve.lock.Unlock()

	go fwd.run(aggChan, aggDFChan, specialChan, confirmedChan,
		finishedChan)
	return fwd, nil
}

func (fwd *sleeveForwarder) logFrom(sender *net.UDPAddr, args ...interface{}) {
	parts := make([]interface{}, len(args)+1)
	parts[0] = fmt.Sprintf("->[%s|%s]:", sender, fwd.remotePeer)
	copy(parts[1:], args)
	log.Println(parts...)
}

func (fwd *sleeveForwarder) log(args ...interface{}) {
	fwd.lock.RLock()
	remoteAddr := fwd.remoteAddr
	fwd.lock.RUnlock()
	fwd.logFrom(remoteAddr, args...)
}

func (fwd *sleeveForwarder) debug(args ...interface{}) {
	fwd.lock.RLock()
	remoteAddr := fwd.remoteAddr
	fwd.lock.RUnlock()
	fwd.logFrom(remoteAddr, args...)
}

func (fwd *sleeveForwarder) SetListener(listener InterHostForwarderListener) {
	fwd.debug("SetListener", listener)

	fwd.lock.Lock()
	fwd.listener = listener
	fwd.lock.Unlock()

	// Setting the listener confirms that the forwarder is really
	// wanted
	if listener != nil {
		select {
		case fwd.confirmedChan <- struct{}{}:
		case <-fwd.finishedChan:
		}
	}
}

type curriedForward struct {
	fwd *sleeveForwarder
	key ForwardPacketKey
}

func (fwd *sleeveForwarder) Forward(key ForwardPacketKey) FlowOp {
	return curriedForward{fwd, key}
}

func (f curriedForward) Send(frame []byte, dec *EthernetDecoder,
	broadcast bool) {
	fwd := f.fwd
	fwd.lock.RLock()
	haveContact := (fwd.remoteAddr != nil)
	effectivePMTU := fwd.effectivePMTU
	stackFrag := fwd.stackFrag
	fwd.lock.RUnlock()

	if !haveContact {
		fwd.log("Cannot forward frame yet - awaiting contact")
		return
	}

	srcName := f.key.SrcPeer.NameByte
	dstName := f.key.DstPeer.NameByte

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
	if dec.DF() {
		if !frameTooBig(frame, effectivePMTU) {
			fwd.aggregate(fwd.aggregatorDFChan, srcName, dstName,
				frame)
			return
		}

		// Why do we need an explicit broadcast hint here,
		// rather than just checking the frame for a broadcast
		// destination MAC address?  Because even
		// non-broadcast frames can be broadcast, if the
		// destination MAC was not in our MAC cache.
		if broadcast {
			log.Printf("dropping too big DF broadcast frame (%v -> %v): PMTU= %v\n", dec.IP.DstIP, dec.IP.SrcIP, effectivePMTU)
			return
		}

		// Send an ICMP back to where the frame came from
		fn, err := dec.makeICMPFragNeeded(effectivePMTU)
		if err != nil {
			log.Println(err)
			return
		}

		dec.DecodeLayers(fn)

		// The frag-needed packet does not have DF set, so the
		// potential recursion here is bounded.
		fwd.sleeve.sendToConsumer(f.key.DstPeer, f.key.SrcPeer, fn, dec)
		return
	}

	if stackFrag || len(dec.decoded) < 2 {
		fwd.aggregate(fwd.aggregatorChan, srcName, dstName, frame)
		return
	}

	// Don't have trustworthy stack, so we're going to have to
	// send it DF in any case.
	if !frameTooBig(frame, effectivePMTU) {
		fwd.aggregate(fwd.aggregatorDFChan, srcName, dstName, frame)
		return
	}

	fwd.sleeve.logFrame("Fragmenting", frame, &dec.Eth)

	// We can't trust the stack to fragment, we have IP, and we
	// have a frame that's too big for the MTU, so we have to
	// fragment it ourself.
	checkWarn(fragment(dec.Eth, dec.IP, effectivePMTU,
		func(segFrame []byte) {
			fwd.aggregate(fwd.aggregatorDFChan, srcName, dstName,
				segFrame)
		}))
}

func (fwd *sleeveForwarder) aggregate(ch chan<- aggregatorFrame, src []byte,
	dst []byte, frame []byte) {
	select {
	case ch <- aggregatorFrame{src, dst, frame}:
	case <-fwd.finishedChan:
	}
}

func fragment(eth layers.Ethernet, ip layers.IPv4, pmtu int,
	forward func([]byte)) error {
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
		err := gopacket.SerializeLayers(buf, opts, &eth, &ip,
			&segPayload)
		if err != nil {
			return err
		}

		forward(buf.Bytes())
	}
	return nil
}

func frameTooBig(frame []byte, effectivePMTU int) bool {
	// We capture/forward complete ethernet frames. Therefore the
	// frame length includes the ethernet header. However, MTUs
	// operate at the IP layer and thus do not include the ethernet
	// header. To put it another way, when a sender that was told an
	// MTU of M sends an IP packet of exactly that length, we will
	// capture/forward M + EthernetOverhead bytes of data.
	return len(frame) > effectivePMTU+EthernetOverhead
}

func (fwd *sleeveForwarder) ControlMessage(msg []byte) {
	select {
	case fwd.specialChan <- specialFrame{nil, msg}:
	case <-fwd.finishedChan:
	}
}

func (fwd *sleeveForwarder) Close() {
	sleeve := fwd.sleeve
	sleeve.lock.Lock()
	if sleeve.forwarders[fwd.remotePeer.Name] == fwd {
		delete(sleeve.forwarders, fwd.remotePeer.Name)
	}
	sleeve.lock.Unlock()
	fwd.SetListener(nil)

	// Tell the forwarder goroutine to finish.  We don't need to
	// wait for it.
	close(fwd.confirmedChan)
}

func (fwd *sleeveForwarder) run(aggChan <-chan aggregatorFrame,
	aggDFChan <-chan aggregatorFrame,
	specialChan <-chan specialFrame,
	confirmedChan <-chan struct{},
	finishedChan chan<- struct{}) {
	defer close(finishedChan)

	var err error
loop:
	for err == nil {
		select {
		case frame := <-aggChan:
			err = fwd.aggregateAndSend(frame, aggChan,
				fwd.crypto.Enc, fwd.sleeve,
				MaxUDPPacketSize-UDPOverhead)

		case frame := <-aggDFChan:
			err = fwd.aggregateAndSend(frame, aggDFChan,
				fwd.crypto.EncDF, fwd.senderDF, fwd.maxPayload)

		case special := <-specialChan:
			err = fwd.handleSpecialFrame(special)

		case _, ok := <-confirmedChan:
			if !ok {
				// specialChan is closed to indicate
				// the forwarder is being closed
				break loop
			}

			err = fwd.confirmed()

		case <-timerChan(fwd.heartbeatTimer):
			err = fwd.sendHeartbeat()

		case <-timerChan(fwd.heartbeatTimeout):
			err = fmt.Errorf("timed out waiting for UDP heartbeat")

		case <-tickerChan(fwd.fragTestTicker):
			err = fwd.sendFragTest()

		case <-timerChan(fwd.pmtuTestTimeout):
			err = fwd.handlePMTUTestFailure()
		}
	}

	if fwd.heartbeatTimer != nil {
		fwd.heartbeatTimer.Stop()
	}
	if fwd.heartbeatTimeout != nil {
		fwd.heartbeatTimeout.Stop()
	}
	if fwd.fragTestTicker != nil {
		fwd.fragTestTicker.Stop()
	}
	if fwd.pmtuTestTimeout != nil {
		fwd.pmtuTestTimeout.Stop()
	}

	checkWarn(fwd.senderDF.close())

	fwd.lock.RLock()
	defer fwd.lock.RUnlock()
	if fwd.listener != nil {
		fwd.listener.Error(err)
	}
}

func (fwd *sleeveForwarder) aggregateAndSend(frame aggregatorFrame,
	aggChan <-chan aggregatorFrame, enc Encryptor, sender udpSender,
	limit int) error {
	// Give up after processing N frames, to avoid starving the
	// other activities of the forwarder goroutine.
	i := 0

	for {
		// Adding the first frame to an empty buffer
		if !fits(frame, enc, limit) {
			fwd.log("Dropping too big frame during forwarding: frame len", len(frame.frame), ", limit ", limit)
			return nil
		}

		for {
			enc.AppendFrame(frame.src, frame.dst, frame.frame)
			i++

			gotOne := false
			if i < 100 {
				select {
				case frame = <-aggChan:
					gotOne = true
				default:
				}
			}

			if !gotOne {
				return fwd.flushEncryptor(enc, sender)
			}

			// Accumulate frames until doing so would
			// exceed the PMTU.  Even in the non-DF case,
			// it doesn't seem worth adding a frame where
			// that would lead to fragmentation,
			// potentially delaying or risking other
			// frames.
			if !fits(frame, enc, fwd.maxPayload) {
				break
			}
		}

		if err := fwd.flushEncryptor(enc, sender); err != nil {
			return err
		}
	}
}

func fits(frame aggregatorFrame, enc Encryptor, limit int) bool {
	return enc.TotalLen()+enc.FrameOverhead()+len(frame.frame) <= limit
}

func (fwd *sleeveForwarder) flushEncryptor(enc Encryptor,
	sender udpSender) error {
	msg, err := enc.Bytes()
	if err != nil {
		return err
	}

	return fwd.processSendError(sender.send(msg, fwd.remoteAddr))
}

func (fwd *sleeveForwarder) sendSpecial(enc Encryptor, sender udpSender,
	data []byte) error {
	enc.AppendFrame(fwd.sleeve.localPeerBin, fwd.remotePeerBin, data)
	return fwd.flushEncryptor(enc, sender)
}

func (fwd *sleeveForwarder) handleSpecialFrame(special specialFrame) error {
	if special.sender == nil {
		return fwd.handleControlMsg(special.frame)
	}

	// The special frame types are distinguished by length
	switch len(special.frame) {
	case EthernetOverhead + 8:
		return fwd.handleHeartbeat(special)

	case FragTestSize:
		return fwd.handleFragTest(special.frame)

	default:
		return fwd.handlePMTUTest(special.frame)
	}
}

const (
	HeartbeatAck = iota
	FragTestAck
	PMTUTestAck
)

func (fwd *sleeveForwarder) handleControlMsg(msg []byte) error {
	if len(msg) == 0 {
		fwd.log("Received zero-length control message")
		return nil
	}

	switch msg[0] {
	case HeartbeatAck:
		return fwd.handleHeartbeatAck()

	case FragTestAck:
		return fwd.handleFragTestAck()

	case PMTUTestAck:
		return fwd.handlePMTUTestAck(msg)

	default:
		fwd.log("Ignoring unknown control message:", msg[0])
		return nil
	}
}

func (fwd *sleeveForwarder) confirmed() error {
	fwd.debug("confirmed")

	if fwd.heartbeatInterval != 0 {
		// already confirmed
		return nil
	}

	// heartbeatInterval flags that we want to send heartbeats,
	// even if we don't do sendHeartbeat() yet due to lacking the
	// remote address.
	fwd.heartbeatInterval = time.Duration(FastHeartbeat)
	if fwd.remoteAddr != nil {
		if err := fwd.sendHeartbeat(); err != nil {
			return err
		}
	}

	fwd.heartbeatTimeout = time.NewTimer(HeartbeatTimeout)
	return nil
}

func (fwd *sleeveForwarder) sendHeartbeat() error {
	fwd.debug("sendHeartbeat")

	// Prime the timer for the next heartbeat.  We don't use a
	// ticker because the interval is not constant.
	fwd.heartbeatTimer = setTimer(fwd.heartbeatTimer, fwd.heartbeatInterval)

	buf := make([]byte, EthernetOverhead+8)
	binary.BigEndian.PutUint64(buf[EthernetOverhead:], fwd.uid)
	return fwd.sendSpecial(fwd.crypto.EncDF, fwd.senderDF, buf)
}

func (fwd *sleeveForwarder) handleHeartbeat(special specialFrame) error {
	uid := binary.BigEndian.Uint64(special.frame[EthernetOverhead:])
	if uid != fwd.uid {
		return nil
	}

	fwd.debug("handleHeartbeat")

	if fwd.remoteAddr == nil {
		fwd.setRemoteAddr(special.sender)
		if fwd.heartbeatInterval != time.Duration(0) {
			if err := fwd.sendHeartbeat(); err != nil {
				return err
			}
		}
	} else if !udpAddrsEqual(fwd.remoteAddr, special.sender) {
		fwd.log("Peer UDP address changed to", special.sender)
		fwd.setRemoteAddr(special.sender)
	}

	if !fwd.ackedHeartbeat {
		fwd.ackedHeartbeat = true
		if err := fwd.sendControlMsg([]byte{HeartbeatAck}); err != nil {
			return err
		}
	}

	fwd.heartbeatTimeout.Reset(HeartbeatTimeout)
	return nil
}

func (fwd *sleeveForwarder) setRemoteAddr(addr *net.UDPAddr) {
	// Although we don't need to lock when reading remoteAddr,
	// because this thread is the only one that modifies
	// remoteAddr, we do need to lock when writing it, because
	// memory models.
	fwd.lock.Lock()
	fwd.remoteAddr = addr
	fwd.lock.Unlock()
}

func (fwd *sleeveForwarder) handleHeartbeatAck() error {
	// The connection is nowregarded as established
	fwd.notifyEstablished()

	if fwd.heartbeatInterval != SlowHeartbeat {
		fwd.heartbeatInterval = SlowHeartbeat
		if fwd.heartbeatTimer != nil {
			fwd.heartbeatTimer.Reset(fwd.heartbeatInterval)
		}
	}

	fwd.fragTestTicker = time.NewTicker(FragTestInterval)
	if err := fwd.sendFragTest(); err != nil {
		return err
	}

	// Send a large frame down the DF channel in order to prompt
	// PMTU discovery to start.
	return fwd.sendSpecial(fwd.crypto.EncDF, fwd.senderDF,
		make([]byte, PMTUDiscoverySize))
}

func (fwd *sleeveForwarder) notifyEstablished() {
	fwd.lock.RLock()
	defer fwd.lock.RUnlock()
	if fwd.listener != nil {
		fwd.listener.Established()
	}
}

func (fwd *sleeveForwarder) sendFragTest() error {
	fwd.debug("sendFragTest")
	fwd.stackFrag = false
	return fwd.sendSpecial(fwd.crypto.Enc, fwd.sleeve,
		make([]byte, FragTestSize))
}

func (fwd *sleeveForwarder) handleFragTest(frame []byte) error {
	if !allZeros(frame) {
		return nil
	}

	return fwd.sendControlMsg([]byte{FragTestAck})
}

func (fwd *sleeveForwarder) handleFragTestAck() error {
	fwd.debug("handleFragTestAck")
	fwd.stackFrag = true
	return nil
}

func (fwd *sleeveForwarder) processSendError(err error) error {
	if mtbe, ok := err.(msgTooBigError); ok {
		epmtu := mtbe.PMTU - fwd.overhead()
		if fwd.epmtuCandidate != 0 && epmtu >= fwd.epmtuCandidate {
			return nil
		}

		fwd.epmtuHighestGood = 8
		fwd.epmtuLowestBad = epmtu + 1
		fwd.epmtuCandidate = epmtu
		fwd.pmtuTestsSent = 0
		fwd.maxPayload = mtbe.PMTU - UDPOverhead
		fwd.effectivePMTU = epmtu
		return fwd.sendPMTUTest()
	}

	return err
}

// How many bytes of overhead it takes to turn an IP packet on the
// overlay network into an encapsulated packet on the underlay network
func (fwd *sleeveForwarder) overhead() int {
	return UDPOverhead + fwd.crypto.EncDF.PacketOverhead() +
		fwd.crypto.EncDF.FrameOverhead() + EthernetOverhead
}

func (fwd *sleeveForwarder) sendPMTUTest() error {
	fwd.debug("sendPMTUTest: epmtu candidate", fwd.epmtuCandidate)
	err := fwd.sendSpecial(fwd.crypto.EncDF, fwd.senderDF,
		make([]byte, fwd.epmtuCandidate+EthernetOverhead))
	if err != nil {
		return err
	}

	fwd.pmtuTestTimeout = setTimer(fwd.pmtuTestTimeout,
		PMTUVerifyTimeout<<fwd.pmtuTestsSent)
	fwd.pmtuTestsSent++
	return nil
}

func (fwd *sleeveForwarder) handlePMTUTest(frame []byte) error {
	buf := make([]byte, 3)
	buf[0] = PMTUTestAck
	binary.BigEndian.PutUint16(buf[1:], uint16(len(frame)-EthernetOverhead))
	return fwd.sendControlMsg(buf)
}

func (fwd *sleeveForwarder) handlePMTUTestAck(msg []byte) error {
	if len(msg) < 3 {
		fwd.log("Received truncated PMTUTestAck")
		return nil
	}

	epmtu := int(binary.BigEndian.Uint16(msg[1:]))
	fwd.debug("handlePMTUTestAck: for epmtu candidate", epmtu)
	if epmtu != fwd.epmtuCandidate {
		return nil
	}

	fwd.epmtuHighestGood = epmtu
	return fwd.searchEPMTU()
}

func (fwd *sleeveForwarder) handlePMTUTestFailure() error {
	if fwd.pmtuTestsSent < PMTUVerifyAttempts {
		return fwd.sendPMTUTest()
	}

	fwd.debug("handlePMTUTestFailure")
	fwd.epmtuLowestBad = fwd.epmtuCandidate
	return fwd.searchEPMTU()
}

func (fwd *sleeveForwarder) searchEPMTU() error {
	fwd.debug("searchEPMTU:", fwd.epmtuHighestGood, fwd.epmtuLowestBad)
	if fwd.epmtuHighestGood+1 >= fwd.epmtuLowestBad {
		epmtu := fwd.epmtuHighestGood
		fwd.log("Effective PMTU verified at", epmtu)

		if fwd.pmtuTestTimeout != nil {
			fwd.pmtuTestTimeout.Stop()
			fwd.pmtuTestTimeout = nil
		}

		fwd.epmtuCandidate = 0
		fwd.maxPayload = epmtu + fwd.overhead() - UDPOverhead
		fwd.effectivePMTU = epmtu
		return nil
	}

	fwd.epmtuCandidate = (fwd.epmtuHighestGood + fwd.epmtuLowestBad) / 2
	fwd.pmtuTestsSent = 0
	return fwd.sendPMTUTest()
}

type udpSenderDF struct {
	ipBuf     gopacket.SerializeBuffer
	opts      gopacket.SerializeOptions
	udpHeader *layers.UDP
	localIP   net.IP
	remoteIP  net.IP
	socket    *net.IPConn
}

func newUDPSenderDF(localIP net.IP, localPort int) *udpSenderDF {
	return &udpSenderDF{
		ipBuf: gopacket.NewSerializeBuffer(),
		opts: gopacket.SerializeOptions{
			FixLengths: true,
			// UDP header is calculated with a phantom IP
			// header. Yes, it's totally nuts. Thankfully,
			// for UDP over IPv4, the checksum is
			// optional. It's not optional for IPv6, but
			// we'll ignore that for now. TODO
			ComputeChecksums: false,
		},
		udpHeader: &layers.UDP{SrcPort: layers.UDPPort(localPort)},
		localIP:   localIP,
	}
}

func (sender *udpSenderDF) dial() error {
	if sender.socket != nil {
		if err := sender.socket.Close(); err != nil {
			return err
		}

		sender.socket = nil
	}

	laddr := &net.IPAddr{IP: sender.localIP}
	raddr := &net.IPAddr{IP: sender.remoteIP}
	s, err := net.DialIP("ip4:UDP", laddr, raddr)

	f, err := s.File()
	if err != nil {
		return err
	}

	defer f.Close()

	// This makes sure all packets we send out have DF set on them.
	err = syscall.SetsockoptInt(int(f.Fd()), syscall.IPPROTO_IP,
		syscall.IP_MTU_DISCOVER, syscall.IP_PMTUDISC_DO)
	if err != nil {
		return err
	}

	sender.socket = s
	return nil
}

func (sender *udpSenderDF) send(msg []byte, raddr *net.UDPAddr) error {
	// Ensure we have a socket sending to the right IP address
	if sender.socket == nil || !bytes.Equal(sender.remoteIP, raddr.IP) {
		sender.remoteIP = raddr.IP
		if err := sender.dial(); err != nil {
			return err
		}
	}

	sender.udpHeader.DstPort = layers.UDPPort(raddr.Port)
	payload := gopacket.Payload(msg)
	err := gopacket.SerializeLayers(sender.ipBuf, sender.opts,
		sender.udpHeader, &payload)
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

	log.Println("EMSGSIZE on send, expecting PMTU update (IP packet was",
		len(packet), "bytes, payload was", len(msg), "bytes)")
	pmtu, err := syscall.GetsockoptInt(int(f.Fd()), syscall.IPPROTO_IP,
		syscall.IP_MTU)
	if err != nil {
		return err
	}

	return msgTooBigError{PMTU: pmtu}
}

type msgTooBigError struct {
	PMTU int // actual pmtu, i.e. what the kernel told us
}

func (mtbe msgTooBigError) Error() string {
	return fmt.Sprint("Msg too big error. PMTU is ", mtbe.PMTU)
}

func (sender *udpSenderDF) close() error {
	if sender.socket == nil {
		return nil
	}

	return sender.socket.Close()
}

func udpAddrsEqual(a *net.UDPAddr, b *net.UDPAddr) bool {
	return bytes.Equal(a.IP, b.IP) && a.Port == b.Port && a.Zone == b.Zone
}

func allZeros(s []byte) bool {
	for _, b := range s {
		if b != byte(0) {
			return false
		}
	}

	return true
}

func setTimer(timer *time.Timer, d time.Duration) *time.Timer {
	if timer == nil {
		return time.NewTimer(d)
	}

	timer.Reset(d)
	return timer

}

func timerChan(timer *time.Timer) <-chan time.Time {
	if timer != nil {
		return timer.C
	}
	return nil
}
