package router

import (
	"fmt"
)

type GossipData interface {
	Encode() [][]byte
	Merge(GossipData)
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
	OnGossipBroadcast(update []byte) (GossipData, error)
	// return state of everything we know; gets called periodically
	Gossip() GossipData
	// merge received data into state and return "everything new I've
	// just learnt", or nil if nothing in the received data was new
	OnGossip(update []byte) (GossipData, error)
}

// Accumulates GossipData that needs to be sent to one destination,
// and sends it when possible.
type GossipSender struct {
	send func(GossipData)
	cell chan GossipData
	// for testing
	sent    bool
	flushch chan chan bool
}

func NewGossipSender(send func(GossipData)) *GossipSender {
	cell := make(chan GossipData, 1)
	flushch := make(chan chan bool)
	sender := &GossipSender{
		send:    send,
		cell:    cell,
		flushch: flushch,
	}
	go sender.run()
	return sender
}

func (sender *GossipSender) run() {
	for {
		select {
		case pending := <-sender.cell:
			if pending == nil { // receive zero value when chan is closed
				return
			}
			sender.send(pending)
			sender.sent = true
		case ch := <-sender.flushch:
			// send anything pending, then reply back whether we sent
			// anything since previous flush
			select {
			case pending := <-sender.cell:
				sender.send(pending)
				sender.sent = true
			default:
			}
			ch <- sender.sent
			sender.sent = false
		}
	}
}

func (sender *GossipSender) Send(data GossipData) {
	// NB: this must not be invoked concurrently
	select {
	case pending := <-sender.cell:
		pending.Merge(data)
		sender.cell <- pending
	default:
		sender.cell <- data
	}
}

func (sender *GossipSender) Stop() {
	close(sender.cell)
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
			channel.Send(router.Ourself.Name, gossip)
		}
	}
}

func (router *Router) SendAllGossipDown(conn Connection) {
	for channel := range router.gossipChannelSet() {
		if gossip := channel.gossiper.Gossip(); gossip != nil {
			channel.SendDown(conn, channel.gossiper.Gossip())
		}
	}
}

// for testing

func (router *Router) sendPendingGossip() bool {
	sentSomething := false
	for channel := range router.gossipChannelSet() {
		channel.Lock()
		for _, sender := range channel.senders {
			sentSomething = sender.flush() || sentSomething
		}
		for _, sender := range channel.broadcasters {
			sentSomething = sender.flush() || sentSomething
		}
		channel.Unlock()
	}
	return sentSomething
}

func (sender *GossipSender) flush() bool {
	ch := make(chan bool)
	sender.flushch <- ch
	return <-ch
}
