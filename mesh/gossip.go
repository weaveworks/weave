package mesh

import (
	"fmt"
	"sync"
)

type GossipData interface {
	Encode() [][]byte
	Merge(GossipData) GossipData
}

type Gossip interface {
	// specific message from one peer to another
	// intermediate peers relay it using unicast topology.
	GossipUnicast(dstPeerName PeerName, msg []byte) error
	// send gossip to every peer, relayed using broadcast topology.
	GossipBroadcast(update GossipData) error
}

type Gossiper interface {
	OnGossipUnicast(sender PeerName, msg []byte) error
	// merge received data into state and return a representation of
	// the received data, for further propagation
	OnGossipBroadcast(sender PeerName, update []byte) (GossipData, error)
	// return state of everything we know; gets called periodically
	Gossip() GossipData
	// merge received data into state and return "everything new I've
	// just learnt", or nil if nothing in the received data was new
	OnGossip(update []byte) (GossipData, error)
}

// Accumulates GossipData that needs to be sent to one destination,
// and sends it when possible.
type GossipSender struct {
	sync.Mutex
	makeMsg          func(msg []byte) ProtocolMsg
	makeBroadcastMsg func(srcName PeerName, msg []byte) ProtocolMsg
	sender           ProtocolSender
	gossip           GossipData
	broadcasts       map[PeerName]GossipData
	more             chan<- struct{}
	flush            chan<- chan<- bool // for testing
}

func NewGossipSender(makeMsg func(msg []byte) ProtocolMsg, makeBroadcastMsg func(srcName PeerName, msg []byte) ProtocolMsg, sender ProtocolSender, stop <-chan struct{}) *GossipSender {
	more := make(chan struct{}, 1)
	flush := make(chan chan<- bool)
	s := &GossipSender{
		makeMsg:          makeMsg,
		makeBroadcastMsg: makeBroadcastMsg,
		sender:           sender,
		broadcasts:       make(map[PeerName]GossipData),
		more:             more,
		flush:            flush}
	go s.run(stop, more, flush)
	return s
}

func (s *GossipSender) run(stop <-chan struct{}, more <-chan struct{}, flush <-chan chan<- bool) {
	sent := false
	for {
		select {
		case <-stop:
			return
		case <-more:
			sentSomething, err := s.deliver(stop)
			if err != nil {
				return
			}
			sent = sent || sentSomething
		case ch := <-flush: // for testing
			// send anything pending, then reply back whether we sent
			// anything since previous flush
			select {
			case <-more:
				sentSomething, err := s.deliver(stop)
				if err != nil {
					return
				}
				sent = sent || sentSomething
			default:
			}
			ch <- sent
			sent = false
		}
	}
}

func (s *GossipSender) deliver(stop <-chan struct{}) (bool, error) {
	sent := false
	// We must not hold our lock when sending, since that would block
	// the callers of Send/Broadcast while we are stuck waiting for
	// network congestion to clear. So we pick and send one piece of
	// data at a time, only holding the lock during the picking.
	for {
		select {
		case <-stop:
			return sent, nil
		default:
		}
		data, makeProtocolMsg := s.pick()
		if data == nil {
			return sent, nil
		}
		for _, msg := range data.Encode() {
			if err := s.sender.SendProtocolMsg(makeProtocolMsg(msg)); err != nil {
				return sent, err
			}
		}
		sent = true
	}
}

func (s *GossipSender) pick() (data GossipData, makeProtocolMsg func(msg []byte) ProtocolMsg) {
	s.Lock()
	defer s.Unlock()
	switch {
	case s.gossip != nil: // usually more important than broadcasts
		data = s.gossip
		makeProtocolMsg = s.makeMsg
		s.gossip = nil
	case len(s.broadcasts) > 0:
		for srcName, d := range s.broadcasts {
			data = d
			makeProtocolMsg = func(msg []byte) ProtocolMsg { return s.makeBroadcastMsg(srcName, msg) }
			delete(s.broadcasts, srcName)
			break
		}
	}
	return
}

func (s *GossipSender) Send(data GossipData) {
	s.Lock()
	defer s.Unlock()
	if s.empty() {
		defer s.prod()
	}
	if s.gossip == nil {
		s.gossip = data
	} else {
		s.gossip = s.gossip.Merge(data)
	}
}

func (s *GossipSender) Broadcast(srcName PeerName, data GossipData) {
	s.Lock()
	defer s.Unlock()
	if s.empty() {
		defer s.prod()
	}
	d, found := s.broadcasts[srcName]
	if !found {
		s.broadcasts[srcName] = data
	} else {
		s.broadcasts[srcName] = d.Merge(data)
	}
}

func (s *GossipSender) empty() bool { return s.gossip == nil && len(s.broadcasts) == 0 }

func (s *GossipSender) prod() {
	select {
	case s.more <- void:
	default:
	}
}

// for testing
func (s *GossipSender) Flush() bool {
	ch := make(chan bool)
	s.flush <- ch
	return <-ch
}

type GossipSenders struct {
	sync.Mutex
	sender  ProtocolSender
	stop    <-chan struct{}
	senders map[string]*GossipSender
}

func NewGossipSenders(sender ProtocolSender, stop <-chan struct{}) *GossipSenders {
	return &GossipSenders{sender: sender, stop: stop, senders: make(map[string]*GossipSender)}
}

func (gs *GossipSenders) Sender(channelName string, makeGossipSender func(sender ProtocolSender, stop <-chan struct{}) *GossipSender) *GossipSender {
	gs.Lock()
	defer gs.Unlock()
	s, found := gs.senders[channelName]
	if !found {
		s = makeGossipSender(gs.sender, gs.stop)
		gs.senders[channelName] = s
	}
	return s
}

// for testing
func (gs *GossipSenders) Flush() bool {
	sent := false
	gs.Lock()
	defer gs.Unlock()
	for _, sender := range gs.senders {
		sent = sender.Flush() || sent
	}
	return sent
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
		if gossip := channel.gossiper.Gossip(); gossip != nil {
			channel.Send(gossip)
		}
	}
}

func (router *Router) SendAllGossipDown(conn Connection) {
	for channel := range router.gossipChannelSet() {
		if gossip := channel.gossiper.Gossip(); gossip != nil {
			channel.SendDown(conn, gossip)
		}
	}
}

// for testing

func (router *Router) sendPendingGossip() bool {
	sentSomething := false
	for conn := range router.Ourself.Connections() {
		sentSomething = conn.(GossipConnection).GossipSenders().Flush() || sentSomething
	}
	return sentSomething
}
