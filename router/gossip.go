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

func (c *GossipChannel) GossipMsg(buf []byte) {
	c.localPeer.ForEachConnection(func(_ PeerName, conn Connection) {
		if conn.Established() {
			peerName := c.localPeer.Name.Bin()
			nameLenByte := []byte{byte(len(peerName))}
			msg := Concat([]byte{ProtocolGossip}, uint32slice(c.hash), nameLenByte, peerName, buf)
			conn.(*LocalConnection).SendTCP(msg)
		}
	})
}

func (c *GossipChannel) GossipUnicast(dstPeerName PeerName, buf []byte) error {
	srcPeerByte := c.localPeer.Name.Bin()
	nameLenByte := []byte{byte(len(srcPeerByte))}
	dstPeerByte := dstPeerName.Bin()
	dstNameLenByte := []byte{byte(len(dstPeerByte))}
	msg := Concat([]byte{ProtocolGossipUnicast}, uint32slice(c.hash), nameLenByte, srcPeerByte, dstNameLenByte, dstPeerByte, buf)
	return c.localPeer.RelayGossipTo(dstPeerName, msg)
}

func (peer *LocalPeer) RelayGossipTo(dstPeerName PeerName, msg []byte) error {
	if relayPeerName, found := peer.Router.Routes.Unicast(dstPeerName); !found {
		log.Println("[gossip] unknown relay destination:", dstPeerName)
		return nil // ?
	} else if conn, found := peer.ConnectionTo(relayPeerName); !found {
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
	c.localPeer.RelayGossipBroadcast(c.localPeer.Name, msg)
	return nil // ?
}

func (peer *LocalPeer) RelayGossipBroadcast(srcName PeerName, msg []byte) {
	if srcPeer, found := peer.Router.Peers.Fetch(srcName); found {
		for _, conn := range peer.NextBroadcastHops(srcPeer) {
			conn.SendTCP(msg)
		}
	} else {
		log.Println("[gossip] unable to relay broadcast from unknown peer", srcName)
	}
}
