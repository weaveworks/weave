package mesh

import (
	"fmt"
	"sync"
)

type Gossip interface {
	// specific message from one peer to another
	// intermediate peers relay it using unicast topology.
	GossipUnicast(dstPeerName PeerName, msg []byte) error
	// send gossip to every peer, relayed using broadcast topology.
	GossipBroadcast(update []byte) error
}

type Gossiper interface {
	OnGossipUnicast(sender PeerName, msg []byte) error
	// merge received data into state
	OnGossipBroadcast(sender PeerName, update []byte) error
	// return state of everything we know; gets called periodically
	Gossip() []byte
	// merge received data into state and return "everything new I've
	// just learnt", or nil if nothing in the received data was new
	OnGossip(update []byte) ([]byte, error)
}

type GossipSender struct {
	sync.Mutex
	sender   ProtocolSender
	gossip   func() *ProtocolMsg
	more     chan<- struct{}
	buf      []ProtocolMsg
	bufSz    int // size of all messages in buf
	bufSzMax int
	sendAll  bool
	flushch  chan chan bool // for testing
}

// Scaling factor for calculating the maximum buffer size from the
// size of the last complete gossip.
const bufSzMaxScale = 1

func NewGossipSender(sender ProtocolSender, stop <-chan struct{}, gossip func() *ProtocolMsg) *GossipSender {
	more := make(chan struct{}, 1)
	flushch := make(chan chan bool)
	s := &GossipSender{sender: sender, gossip: gossip, more: more, flushch: flushch}
	go s.run(stop, more)
	return s
}

func (s *GossipSender) run(stop <-chan struct{}, more <-chan struct{}) {
	sent := false
	for {
		select {
		case <-stop:
			return
		case <-more:
			sentSomething, err := s.send()
			if err != nil {
				return
			}
			sent = sentSomething || sent
		case ch := <-s.flushch: // for testing
			// send anything pending, then reply back whether we sent
			// anything since previous flush
			for empty := false; !empty; {
				select {
				case <-more:
					sentSomething, _ := s.send()
					sent = sentSomething || sent
				default:
					empty = true
				}
			}
			ch <- sent
			sent = false
		}
	}
}

func (s *GossipSender) send() (bool, error) {
	sent := false
	for {
		msg := s.dequeue()
		if msg == nil {
			return sent, nil
		}
		if err := s.sender.SendProtocolMsg(*msg); err != nil {
			return sent, err
		}
		sent = true
	}
}

func (s *GossipSender) SendGossip(msg ProtocolMsg) {
	s.Lock()
	defer s.Unlock()
	if s.sendAll {
		return
	}
	if s.bufSzMax == 0 || s.bufSz+len(msg.msg) <= s.bufSzMax {
		s.buf = append(s.buf, msg)
		s.bufSz += len(msg.msg)
		if len(s.buf) == 1 {
			s.prod()
		}
		return
	}
	s.clear()
	s.prod()
}

func (s *GossipSender) SendAllGossip() {
	s.Lock()
	defer s.Unlock()
	if s.sendAll {
		return
	}
	s.clear()
	s.prod()
}

func (s *GossipSender) clear() {
	s.buf = nil
	s.bufSz = 0
	s.sendAll = true
}

func (s *GossipSender) prod() {
	select {
	case s.more <- void:
	default:
	}
}

func (s *GossipSender) dequeue() *ProtocolMsg {
	s.Lock()
	defer s.Unlock()
	if len(s.buf) > 0 { // NB: s.sendAll -> s.buf == nil
		msg := s.buf[0]
		s.buf = s.buf[1:]
		s.bufSz -= len(msg.msg)
		return &msg
	}
	if !s.sendAll {
		return nil
	}

	// allow more enqueuing
	s.sendAll = false

	// We don't know what obtaining gossip entails, so in order to
	// avoid deadlocks, we must not hold any locks while doing so.
	s.Unlock()
	msg := s.gossip()
	s.Lock()
	if msg == nil {
		s.bufSzMax = 0
	} else {
		s.bufSzMax = bufSzMaxScale * len(msg.msg)
		if s.bufSz > s.bufSzMax {
			s.clear()
		}
	}
	return msg
}

type GossipSenders struct {
	sync.Mutex
	sender  ProtocolSender
	stop    <-chan struct{}
	senders map[string]ProtocolGossipSender
}

func NewGossipSenders(sender ProtocolSender, stop <-chan struct{}) *GossipSenders {
	return &GossipSenders{sender: sender, stop: stop, senders: make(map[string]ProtocolGossipSender)}
}

func (gs *GossipSenders) Sender(channelName string, gossip func() *ProtocolMsg) ProtocolGossipSender {
	gs.Lock()
	defer gs.Unlock()
	s, found := gs.senders[channelName]
	if !found {
		s = NewGossipSender(gs.sender, gs.stop, gossip)
		gs.senders[channelName] = s
	}
	return s
}

type GossipChannels map[string]*GossipChannel

func (router *Router) NewGossip(channelName string, g Gossiper) Gossip {
	channel := NewGossipChannel(channelName, router.Ourself, router.Routes, g)
	router.gossipLock.Lock()
	defer router.gossipLock.Unlock()
	if _, found := router.gossipChannels[channelName]; found {
		checkFatal(fmt.Errorf("[gossip] duplicate channel %s", channelName))
	}
	router.gossipChannels[channelName] = channel
	return channel
}

func (router *Router) gossipChannel(channelName string) *GossipChannel {
	router.gossipLock.RLock()
	channel, found := router.gossipChannels[channelName]
	router.gossipLock.RUnlock()
	if found {
		return channel
	}
	router.gossipLock.Lock()
	defer router.gossipLock.Unlock()
	if channel, found = router.gossipChannels[channelName]; found {
		return channel
	}
	channel = NewGossipChannel(channelName, router.Ourself, router.Routes, &surrogateGossiper)
	channel.log("created surrogate channel")
	router.gossipChannels[channelName] = channel
	return channel
}

func (router *Router) gossipChannelSet() map[*GossipChannel]struct{} {
	channels := make(map[*GossipChannel]struct{})
	router.gossipLock.RLock()
	defer router.gossipLock.RUnlock()
	for _, channel := range router.gossipChannels {
		channels[channel] = void
	}
	return channels
}

func (router *Router) SendAllGossip() {
	for channel := range router.gossipChannelSet() {
		channel.Send()
	}
}

func (router *Router) SendAllGossipDown(conn Connection) {
	for channel := range router.gossipChannelSet() {
		channel.SendDown(conn)
	}
}

// for testing

func (router *Router) sendPendingGossip() bool {
	sentSomething := false
	for channel := range router.gossipChannelSet() {
		for conn := range router.Ourself.Connections() {
			sender := conn.(ProtocolSender).GossipSender(channel.name, channel.gossip)
			sentSomething = sender.(*GossipSender).flush() || sentSomething
		}
	}
	return sentSomething
}

func (s *GossipSender) flush() bool {
	ch := make(chan bool)
	s.flushch <- ch
	return <-ch
}
