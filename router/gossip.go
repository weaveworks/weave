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

func (conn *LocalConnection) handleGossip(payload []byte, onok func(*GossipChannel, PeerName, []byte, *gob.Decoder) error) error {
	decoder := gob.NewDecoder(bytes.NewReader(payload))
	var channelHash uint32
	if err := decoder.Decode(&channelHash); err != nil {
		return err
	}
	channel, found := conn.Router.GossipChannels[channelHash]
	if !found {
		// Don't close the connection on unknown gossip - maybe the sysadmin has
		// upgraded one node in the weave network and intends to do this one shortly.
		conn.log("[gossip] received unknown channel with hash", channelHash)
		return nil
	}
	var srcName PeerName
	if err := decoder.Decode(&srcName); err != nil {
		return err
	}
	if err := onok(channel, srcName, payload, decoder); err != nil {
		return err
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
		channel.relayGossipUnicast(destName, origPayload)
	}
	return nil
}

func deliverGossipBroadcast(channel *GossipChannel, srcName PeerName, origPayload []byte, dec *gob.Decoder) error {
	var payload []byte
	if err := dec.Decode(&payload); err != nil {
		return err
	}
	channel.gossiper.OnGossipBroadcast(payload)
	return channel.relayGossipBroadcast(srcName, origPayload)
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
	return c.relayGossipUnicast(dstPeerName, GobEncode(c.hash, c.ourself.Name, dstPeerName, buf))
}

func (c *GossipChannel) GossipBroadcast(buf []byte) error {
	return c.relayGossipBroadcast(c.ourself.Name, GobEncode(c.hash, c.ourself.Name, buf))
}

func (c *GossipChannel) relayGossipUnicast(dstPeerName PeerName, msg []byte) error {
	if relayPeerName, found := c.ourself.Router.Routes.Unicast(dstPeerName); !found {
		logGossip("unknown relay destination:", dstPeerName)
	} else if conn, found := c.ourself.ConnectionTo(relayPeerName); !found {
		logGossip("unable to find connection to relay peer", relayPeerName)
	} else {
		conn.(ProtocolSender).SendProtocolMsg(ProtocolMsg{ProtocolGossipUnicast, msg})
	}
	return nil
}

func (c *GossipChannel) relayGossipBroadcast(srcName PeerName, msg []byte) error {
	if srcPeer, found := c.ourself.Router.Peers.Fetch(srcName); !found {
		logGossip("unable to relay broadcast from unknown peer", srcName)
	} else {
		protocolMsg := ProtocolMsg{ProtocolGossipBroadcast, msg}
		for _, conn := range c.ourself.NextBroadcastHops(srcPeer) {
			conn.SendProtocolMsg(protocolMsg)
		}
	}
	return nil
}
