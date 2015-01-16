package router

import (
	"log"
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
	localPeer *LocalPeer
	name      string
	hash      uint32
	gossiper  Gossiper
}

func (router *Router) NewGossip(channelName string, g Gossiper) Gossip {
	h := hash(channelName)
	channel := &GossipChannel{router.Ourself, channelName, h, g}
	router.GossipChannels[h] = channel
	return channel
}

func (router *Router) SendAllGossip() {
	for _, c := range router.GossipChannels {
		c.GossipMsg(c.gossiper.Gossip())
	}
}

func (router *Router) SendGossip(channelName string, msg []byte) {
	channelHash := hash(channelName)
	if channel, found := router.GossipChannels[channelHash]; !found {
		log.Println("[gossip] attempt to send on unknown channel:", channelName)
	} else {
		channel.GossipMsg(msg)
	}
}

func (c *GossipChannel) GossipMsg(buf []byte) {
	c.localPeer.ForEachConnection(func(_ PeerName, conn Connection) {
		if conn.Established() {
			peerName := c.localPeer.Name.Bin()
			nameLenByte := []byte{byte(len(peerName))}
			msg := Concat([]byte{ProtocolGossip}, uint32slice(c.hash), nameLenByte, peerName, buf)
			conn.(ConnectionSender).SendTCP(msg)
		}
	})
}

func handleGossip(conn *LocalConnection, msg []byte, onok func(channel *GossipChannel, srcName PeerName, origMsg, payload []byte)) {
	if len(msg) < 10 {
		conn.log("[gossip] received invalid message of length:", len(msg))
		return
	}
	channelHash, payload := decodeGossipChannel(msg[1:])
	if channel, found := conn.Router.GossipChannels[channelHash]; !found {
		conn.log("[gossip] received unknown channel:", channelHash)
	} else {
		srcName, payload := decodePeerName(payload)
		onok(channel, srcName, msg, payload)
	}
}

func deliverGossipUnicast(channel *GossipChannel, srcName PeerName, origMsg, payload []byte) {
	destName, msg := decodePeerName(payload)
	if channel.localPeer.Name == destName {
		channel.gossiper.OnGossipUnicast(srcName, msg)
	} else {
		channel.RelayGossipTo(destName, origMsg)
	}
}

func deliverGossipBroadcast(channel *GossipChannel, srcName PeerName, origMsg, payload []byte) {
	channel.gossiper.OnGossipBroadcast(payload)
	channel.RelayGossipBroadcast(srcName, origMsg)
}

func deliverGossip(channel *GossipChannel, srcName PeerName, origMsg, payload []byte) {
	if newBuf := channel.gossiper.OnGossip(payload); newBuf != nil {
		channel.GossipMsg(newBuf)
	}
}

func (c *GossipChannel) GossipUnicast(dstPeerName PeerName, buf []byte) error {
	srcPeerByte := c.localPeer.Name.Bin()
	nameLenByte := []byte{byte(len(srcPeerByte))}
	dstPeerByte := dstPeerName.Bin()
	dstNameLenByte := []byte{byte(len(dstPeerByte))}
	msg := Concat([]byte{ProtocolGossipUnicast}, uint32slice(c.hash), nameLenByte, srcPeerByte, dstNameLenByte, dstPeerByte, buf)
	return c.RelayGossipTo(dstPeerName, msg)
}

func (c *GossipChannel) RelayGossipTo(dstPeerName PeerName, msg []byte) error {
	if relayPeerName, found := c.localPeer.Router.Routes.Unicast(dstPeerName); !found {
		log.Println("[gossip] unknown relay destination:", dstPeerName)
		return nil // ?
	} else if conn, found := c.localPeer.ConnectionTo(relayPeerName); !found {
		log.Println("[gossip] unable to find connection to relay peer", relayPeerName)
		return nil // ?
	} else {
		conn.(*LocalConnection).SendTCP(msg)
	}
	return nil
}

func (c *GossipChannel) GossipBroadcast(buf []byte) error {
	peerName := c.localPeer.Name.Bin()
	nameLenByte := []byte{byte(len(peerName))}
	msg := Concat([]byte{ProtocolGossipBroadcast}, uint32slice(c.hash), nameLenByte, peerName, buf)
	c.RelayGossipBroadcast(c.localPeer.Name, msg)
	return nil // ?
}

func (c *GossipChannel) RelayGossipBroadcast(srcName PeerName, msg []byte) {
	if srcPeer, found := c.localPeer.Router.Peers.Fetch(srcName); found {
		for _, conn := range c.localPeer.NextBroadcastHops(srcPeer) {
			conn.SendTCP(msg)
		}
	} else {
		log.Println("[gossip] unable to relay broadcast from unknown peer", srcName)
	}
}
