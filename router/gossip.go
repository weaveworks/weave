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

type GossipChannel struct {
	sync.Mutex
	ourself  *LocalPeer
	name     string
	hash     uint32
	gossiper Gossiper
	senders  connectionSenders
}

func (router *Router) NewGossip(channelName string, g Gossiper) Gossip {
	channelHash := hash(channelName)
	channel := &GossipChannel{
		ourself:  router.Ourself,
		name:     channelName,
		hash:     channelHash,
		gossiper: g,
		senders:  make(connectionSenders)}
	router.GossipChannels[channelHash] = channel
	return channel
}

func (router *Router) SendAllGossip() {
	for _, channel := range router.GossipChannels {
		channel.Send(channel.gossiper.Gossip())
	}
}

func (router *Router) SendAllGossipDown(conn Connection) {
	for _, channel := range router.GossipChannels {
		channel.SendDown(conn, channel.gossiper.Gossip())
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

func (c *GossipChannel) deliver(_ PeerName, _ []byte, dec *gob.Decoder) error {
	var payload []byte
	if err := dec.Decode(&payload); err != nil {
		return err
	}
	if data, err := c.gossiper.OnGossip(payload); err != nil {
		return err
	} else if data != nil {
		c.Send(data)
	}
	return nil
}

func (c *GossipChannel) Send(data GossipData) {
	connections := c.ourself.Connections() // do this outside the lock so they don't nest
	retainedSenders := make(connectionSenders)
	c.Lock()
	defer c.Unlock()
	for conn := range connections {
		c.sendDown(conn, data)
		retainedSenders[conn] = c.senders[conn]
		delete(c.senders, conn)
	}
	// stop any senders for connections that are gone
	for _, sender := range c.senders {
		sender.Stop()
	}
	c.senders = retainedSenders
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
	if relayPeerName, found := c.ourself.Router.Routes.UnicastAll(dstPeerName); !found {
		c.log("unknown relay destination:", dstPeerName)
	} else if conn, found := c.ourself.ConnectionTo(relayPeerName); !found {
		c.log("unable to find connection to relay peer", relayPeerName)
	} else {
		conn.(ProtocolSender).SendProtocolMsg(ProtocolMsg{ProtocolGossipUnicast, buf})
	}
	return nil
}

func (c *GossipChannel) relayBroadcast(srcName PeerName, update GossipData) error {
	nextHops := c.ourself.Router.Routes.BroadcastAll(srcName)
	if len(nextHops) == 0 {
		return nil
	}
	protocolMsg := ProtocolMsg{ProtocolGossipBroadcast, GobEncode(c.hash, srcName, update.Encode())}
	// FIXME a single blocked connection can stall us
	for _, conn := range c.ourself.ConnectionsTo(nextHops) {
		conn.(ProtocolSender).SendProtocolMsg(protocolMsg)
	}
	return nil
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
