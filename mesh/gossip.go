package mesh

import (
	"fmt"
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
	send  func(GossipData)
	cell  chan GossipData
	flush chan<- chan<- bool // for testing
}

func NewGossipSender(send func(GossipData)) *GossipSender {
	cell := make(chan GossipData, 1)
	flush := make(chan chan<- bool)
	s := &GossipSender{send: send, cell: cell, flush: flush}
	go s.run(flush)
	return s
}

func (s *GossipSender) run(flush <-chan chan<- bool) {
	sent := false
	for {
		select {
		case pending := <-s.cell:
			if pending == nil { // receive zero value when chan is closed
				return
			}
			s.send(pending)
			sent = true
		case ch := <-flush: // for testing
			// send anything pending, then reply back whether we sent
			// anything since previous flush
			select {
			case pending := <-s.cell:
				s.send(pending)
				sent = true
			default:
			}
			ch <- sent
			sent = false
		}
	}
}

func (s *GossipSender) Send(data GossipData) {
	// NB: this must not be invoked concurrently
	select {
	case pending := <-s.cell:
		s.cell <- pending.Merge(data)
	default:
		s.cell <- data
	}
}

// for testing
func (s *GossipSender) Flush() bool {
	ch := make(chan bool)
	s.flush <- ch
	return <-ch
}

func (s *GossipSender) Stop() {
	close(s.cell)
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
	for channel := range router.gossipChannelSet() {
		channel.Lock()
		for _, sender := range channel.senders {
			sentSomething = sender.Flush() || sentSomething
		}
		for _, sender := range channel.broadcasters {
			sentSomething = sender.Flush() || sentSomething
		}
		channel.Unlock()
	}
	return sentSomething
}
