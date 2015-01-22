package router

import (
	"bytes"
	"encoding/gob"
	"log"
	"time"
)

const (
	GossipInterval   = 3 * time.Second
	TopologyGossipCh = "topology"
)

type Gossip interface {
	// specific message from one peer to another
	// intermediate peers relay it using unicast topology.
	GossipUnicast(dstPeerName PeerName, buf []byte) error
	// send a message to every peer, relayed using broadcast topology.
	GossipBroadcast(buf []byte) error
}

type Gossiper interface {
	OnGossipUnicast(sender PeerName, msg []byte)
	OnGossipBroadcast(msg []byte)
	// Return state of everything we know; gets called periodically
	Gossip() []byte
	// merge in state and return "everything new I've just learnt",
	// or nil if nothing in the received message was new
	OnGossip(buf []byte) []byte
}

type GossipChannel struct {
	ourself  *LocalPeer
	name     string
	hash     uint32
	gossiper Gossiper
}

func (router *Router) NewGossip(channelName string, g Gossiper) Gossip {
	channelHash := hash(channelName)
	channel := &GossipChannel{router.Ourself, channelName, channelHash, g}
	router.GossipChannels[channelHash] = channel
	return channel
}

func logGossip(args ...interface{}) {
	log.Println(append(append([]interface{}{}, "[gossip]:"), args...)...)
}

func (router *Router) SendAllGossip() {
	for _, channel := range router.GossipChannels {
		channel.GossipMsg(channel.gossiper.Gossip())
	}
}

func (router *Router) SendGossip(channelName string, msg []byte) {
	channelHash := hash(channelName)
	if channel, found := router.GossipChannels[channelHash]; !found {
		logGossip("attempt to send on unknown channel:", channelName)
	} else {
		channel.GossipMsg(msg)
	}
}

func (router *Router) SendAllGossipDown(conn Connection) {
	for _, channel := range router.GossipChannels {
		channel.send(channel.gossiper.Gossip(), conn)
	}
}

func (c *GossipChannel) send(buf []byte, conn Connection) {
	msg := ProtocolMsg{ProtocolGossip, GobEncode(c.hash, c.ourself.Name, buf)}
	conn.(ProtocolSender).SendProtocolMsg(msg)
}

func (c *GossipChannel) GossipMsg(buf []byte) {
	c.ourself.ForEachConnection(func(_ PeerName, conn Connection) {
		c.send(buf, conn)
	})
}

func handleGossip(conn *LocalConnection, payload []byte, onok func(channel *GossipChannel, srcName PeerName, origPayload []byte, dec *gob.Decoder) error) error {
	decoder := gob.NewDecoder(bytes.NewReader(payload))
	var channelHash uint32
	if err := decoder.Decode(&channelHash); err != nil {
		conn.Shutdown(err)
		return err
	}
	if channel, found := conn.Router.GossipChannels[channelHash]; !found {
		// Don't close the connection on unknown gossip - maybe the sysadmin has
		// upgraded one node in the weave network and intends to do this one shortly.
		logGossip("received unknown channel:", channelHash, "from ", conn.Remote().Name)
		return nil
	} else {
		var srcName PeerName
		if err := decoder.Decode(&srcName); err == nil {
			conn.Shutdown(err)
			return err
		}
		if err := onok(channel, srcName, payload, decoder); err != nil {
			conn.Shutdown(err)
			return err
		}
	}
	return nil
}

func deliverGossipUnicast(channel *GossipChannel, srcName PeerName, origPayload []byte, dec *gob.Decoder) error {
	var destName PeerName
	if err := dec.Decode(&destName); err != nil {
		return err
	}
	if channel.ourself.Name == destName {
		var payload []byte
		if err := dec.Decode(&payload); err != nil {
			return err
		}
		channel.gossiper.OnGossipUnicast(srcName, payload)
	} else {
		channel.RelayGossipTo(destName, ProtocolMsg{ProtocolGossipUnicast, origPayload})
	}
	return nil
}

func deliverGossipBroadcast(channel *GossipChannel, srcName PeerName, origPayload []byte, dec *gob.Decoder) error {
	var payload []byte
	if err := dec.Decode(&payload); err != nil {
		return err
	}
	channel.gossiper.OnGossipBroadcast(payload)
	channel.RelayGossipBroadcast(srcName, ProtocolMsg{ProtocolGossipBroadcast, origPayload})
	return nil
}

func deliverGossip(channel *GossipChannel, srcName PeerName, _ []byte, dec *gob.Decoder) error {
	var payload []byte
	if err := dec.Decode(&payload); err != nil {
		return err
	}
	if newBuf := channel.gossiper.OnGossip(payload); newBuf != nil {
		channel.GossipMsg(newBuf)
	}
	return nil
}

func (c *GossipChannel) GossipUnicast(dstPeerName PeerName, buf []byte) error {
	msg := ProtocolMsg{ProtocolGossipUnicast, GobEncode(c.hash, c.ourself.Name, dstPeerName, buf)}
	return c.RelayGossipTo(dstPeerName, msg)
}

func (c *GossipChannel) RelayGossipTo(dstPeerName PeerName, msg ProtocolMsg) error {
	if relayPeerName, found := c.ourself.Router.Routes.Unicast(dstPeerName); !found {
		logGossip("unknown relay destination:", dstPeerName)
		return nil // ?
	} else if conn, found := c.ourself.ConnectionTo(relayPeerName); !found {
		logGossip("unable to find connection to relay peer", relayPeerName)
		return nil // ?
	} else {
		conn.(ProtocolSender).SendProtocolMsg(msg)
	}
	return nil
}

func (c *GossipChannel) GossipBroadcast(buf []byte) error {
	msg := ProtocolMsg{ProtocolGossipBroadcast, GobEncode(c.hash, c.ourself.Name, buf)}
	c.RelayGossipBroadcast(c.ourself.Name, msg)
	return nil // ?
}

func (c *GossipChannel) RelayGossipBroadcast(srcName PeerName, msg ProtocolMsg) {
	if srcPeer, found := c.ourself.Router.Peers.Fetch(srcName); found {
		for _, conn := range c.ourself.NextBroadcastHops(srcPeer) {
			conn.SendProtocolMsg(msg)
		}
	} else {
		logGossip("unable to relay broadcast from unknown peer", srcName)
	}
}
