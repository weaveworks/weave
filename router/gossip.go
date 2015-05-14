package router

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"log"
	"sync"
	"time"
)

const GossipInterval = 30 * time.Second

type GossipData interface {
	Encode() []byte
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
}

func NewGossipSender(send func(GossipData)) *GossipSender {
	return &GossipSender{send: send}
}

func (sender *GossipSender) Start() {
	sender.cell = make(chan GossipData, 1)
	go sender.run()
}

func (sender *GossipSender) run() {
	for {
		if pending := <-sender.cell; pending == nil { // receive zero value when chan is closed
			break
		} else {
			sender.send(pending)
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

type connectionSenders map[Connection]*GossipSender
type peerSenders map[PeerName]*GossipSender

type GossipChannel struct {
	sync.Mutex
	ourself      *LocalPeer
	routes       *Routes
	name         string
	hash         uint32
	gossiper     Gossiper
	senders      connectionSenders
	broadcasters peerSenders
}

func (router *Router) NewGossip(channelName string, g Gossiper) Gossip {
	channelHash := hash(channelName)
	channel := &GossipChannel{
		ourself:      router.Ourself,
		routes:       router.Routes,
		name:         channelName,
		hash:         channelHash,
		gossiper:     g,
		senders:      make(connectionSenders),
		broadcasters: make(peerSenders)}
	router.GossipChannels[channelHash] = channel
	return channel
}

func (router *Router) SendAllGossip() {
	for _, channel := range router.GossipChannels {
		if gossip := channel.gossiper.Gossip(); gossip != nil {
			channel.Send(router.Ourself.Name, gossip)
		}
	}
}

func (router *Router) SendAllGossipDown(conn Connection) {
	for _, channel := range router.GossipChannels {
		if gossip := channel.gossiper.Gossip(); gossip != nil {
			channel.SendDown(conn, channel.gossiper.Gossip())
		}
	}
}

func (router *Router) handleGossip(tag ProtocolTag, payload []byte) error {
	decoder := gob.NewDecoder(bytes.NewReader(payload))
	var channelHash uint32
	if err := decoder.Decode(&channelHash); err != nil {
		return err
	}
	channel, found := router.GossipChannels[channelHash]
	if !found {
		return fmt.Errorf("[gossip] received unknown channel with hash %v", channelHash)
	}
	var srcName PeerName
	if err := decoder.Decode(&srcName); err != nil {
		return err
	}
	switch tag {
	case ProtocolGossipUnicast:
		return channel.deliverUnicast(srcName, payload, decoder)
	case ProtocolGossipBroadcast:
		return channel.deliverBroadcast(srcName, payload, decoder)
	case ProtocolGossip:
		return channel.deliver(srcName, payload, decoder)
	}
	return nil
}

func (c *GossipChannel) deliverUnicast(srcName PeerName, origPayload []byte, dec *gob.Decoder) error {
	var destName PeerName
	if err := dec.Decode(&destName); err != nil {
		return err
	}
	if c.ourself.Name != destName {
		return c.relayUnicast(destName, origPayload)
	}
	var payload []byte
	if err := dec.Decode(&payload); err != nil {
		return err
	}
	return c.gossiper.OnGossipUnicast(srcName, payload)
}

func (c *GossipChannel) deliverBroadcast(srcName PeerName, _ []byte, dec *gob.Decoder) error {
	var payload []byte
	if err := dec.Decode(&payload); err != nil {
		return err
	}
	data, err := c.gossiper.OnGossipBroadcast(payload)
	if err != nil || data == nil {
		return err
	}
	return c.relayBroadcast(srcName, data)
}

func (c *GossipChannel) deliver(srcName PeerName, _ []byte, dec *gob.Decoder) error {
	var payload []byte
	if err := dec.Decode(&payload); err != nil {
		return err
	}
	if data, err := c.gossiper.OnGossip(payload); err != nil {
		return err
	} else if data != nil {
		c.Send(srcName, data)
	}
	return nil
}

func (c *GossipChannel) Send(srcName PeerName, data GossipData) {
	// do this outside the lock below so we avoid lock nesting
	c.routes.EnsureRecalculated()
	selectedConnections := make(ConnectionSet)
	for name := range c.routes.RandomNeighbours(srcName) {
		if conn, found := c.ourself.ConnectionTo(name); found {
			selectedConnections[conn] = void
		}
	}
	if len(selectedConnections) == 0 {
		return
	}
	connections := c.ourself.Connections()
	c.Lock()
	defer c.Unlock()
	// GC - randomly (courtesy of go's map iterator) pick some
	// existing entries and stop&remove them if the associated
	// connection is no longer active.  We stop as soon as we
	// encounter a valid entry; the idea being that when there is
	// little or no garbage then this executes close to O(1)[1],
	// whereas when there is lots of garbage we remove it quickly.
	//
	// [1] TODO Unfortunately, due to the desire to avoid nested
	// locks, instead of simply invoking Peer.ConnectionTo(name)
	// below, we have that Peer.Connections() invocation above. That
	// is O(n_our_connections) at best.
	for conn, sender := range c.senders {
		if _, found := connections[conn]; !found {
			delete(c.senders, conn)
			sender.Stop()
		} else {
			break
		}
	}
	for conn := range selectedConnections {
		c.sendDown(conn, data)
	}
}

func (c *GossipChannel) SendDown(conn Connection, data GossipData) {
	c.Lock()
	c.sendDown(conn, data)
	c.Unlock()
}

func (c *GossipChannel) sendDown(conn Connection, data GossipData) {
	sender, found := c.senders[conn]
	if !found {
		sender = NewGossipSender(func(pending GossipData) {
			protocolMsg := ProtocolMsg{ProtocolGossip, GobEncode(c.hash, c.ourself.Name, pending.Encode())}
			conn.(ProtocolSender).SendProtocolMsg(protocolMsg)
		})
		c.senders[conn] = sender
		sender.Start()
	}
	sender.Send(data)
}

func (c *GossipChannel) GossipUnicast(dstPeerName PeerName, msg []byte) error {
	return c.relayUnicast(dstPeerName, GobEncode(c.hash, c.ourself.Name, dstPeerName, msg))
}

func (c *GossipChannel) GossipBroadcast(update GossipData) error {
	return c.relayBroadcast(c.ourself.Name, update)
}

func (c *GossipChannel) relayUnicast(dstPeerName PeerName, buf []byte) error {
	if relayPeerName, found := c.routes.UnicastAll(dstPeerName); !found {
		c.log("unknown relay destination:", dstPeerName)
	} else if conn, found := c.ourself.ConnectionTo(relayPeerName); !found {
		c.log("unable to find connection to relay peer", relayPeerName)
	} else {
		conn.(ProtocolSender).SendProtocolMsg(ProtocolMsg{ProtocolGossipUnicast, buf})
	}
	return nil
}

func (c *GossipChannel) relayBroadcast(srcName PeerName, update GossipData) error {
	names := c.routes.PeerNames() // do this outside the lock so they don't nest
	c.Lock()
	defer c.Unlock()
	// GC - randomly (courtesy of go's map iterator) pick some
	// existing broadcasters and stop&remove them if their source peer
	// is unknown. We stop as soon as we encounter a valid entry; the
	// idea being that when there is little or no garbage then this
	// executes close to O(1)[1], whereas when there is lots of
	// garbage we remove it quickly.
	//
	// [1] TODO Unfortunately, due to the desire to avoid nested
	// locks, instead of simply invoking Peers.Fetch(name) below, we
	// have that Peers.Names() invocation above. That is O(n_peers) at
	// best.
	for name, broadcaster := range c.broadcasters {
		if _, found := names[name]; !found {
			delete(c.broadcasters, name)
			broadcaster.Stop()
		} else {
			break
		}
	}
	broadcaster, found := c.broadcasters[srcName]
	if !found {
		broadcaster = NewGossipSender(func(pending GossipData) { c.sendBroadcast(srcName, pending) })
		c.broadcasters[srcName] = broadcaster
		broadcaster.Start()
	}
	broadcaster.Send(update)
	return nil
}

func (c *GossipChannel) sendBroadcast(srcName PeerName, update GossipData) {
	c.routes.EnsureRecalculated()
	nextHops := c.routes.BroadcastAll(srcName)
	if len(nextHops) == 0 {
		return
	}
	protocolMsg := ProtocolMsg{ProtocolGossipBroadcast, GobEncode(c.hash, srcName, update.Encode())}
	// FIXME a single blocked connection can stall us
	for _, conn := range c.ourself.ConnectionsTo(nextHops) {
		conn.(ProtocolSender).SendProtocolMsg(protocolMsg)
	}
}

func (c *GossipChannel) log(args ...interface{}) {
	log.Println(append(append([]interface{}{}, "[gossip "+c.name+"]:"), args...)...)
}

// for testing

// FIXME this doesn't actually guarantee everything has been sent
// since a GossipSender may be in the process of sending and there is
// no easy way for us to know when that has completed.
func (router *Router) sendPendingGossip() {
	for _, channel := range router.GossipChannels {
		for _, sender := range channel.senders {
			sender.flush()
		}
		for _, sender := range channel.broadcasters {
			sender.flush()
		}
	}
}

func (sender *GossipSender) flush() {
	for {
		select {
		case pending := <-sender.cell:
			sender.send(pending)
		default:
			return
		}
	}
}
