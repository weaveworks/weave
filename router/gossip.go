package router

import (
	"log"
)

// Interface to receive notifications when we spot the presence or absence of a peer
type PeerLifecycle interface {
	OnAlive(uint64)
	OnDead(uint64)
}

type Gossip interface {
	// specific message from one peer to another
	// intermediate peers should relay it using unicast topology.
	GossipUnicast(dstPeerName PeerName, buf []byte) error
	// intended for a state change that everyone should hear about quickly
	// relayed using broadcast topology.
	GossipBroadcast(buf []byte) error
}

type Gossiper interface {
	PeerLifecycle
	OnGossipBroadcast(msg []byte)
	OnGossipUnicast(sender PeerName, msg []byte)
	// Return state of everything we know; intended to be called periodically
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

// contains state for everyone that sending peer knows
// done on an interval; sent by one peer down [all/random subset of] connections
// peers that receive it should examine the info, and if it is broadcast
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

// intended for state from sending peer only
// done when there is a change that everyone should hear about quickly
// peers that receive it should relay it using broadcast topology.
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
		log.Println("Unable to relay gossip from unknown peer", srcName)
	}
}

// specific message from one peer to another
// intermediate peers should relay it using unicast topology.
func (c *GossipChannel) GossipUnicast(dstPeerName PeerName, buf []byte) error {
	srcPeerByte := c.localPeer.Name.Bin()
	nameLenByte := []byte{byte(len(srcPeerByte))}
	dstPeerByte := dstPeerName.Bin()
	dstNameLenByte := []byte{byte(len(dstPeerByte))}
	msg := Concat([]byte{ProtocolGossipUnicast}, uint32slice(c.hash), nameLenByte, srcPeerByte, dstNameLenByte, dstPeerByte, buf)
	return c.localPeer.RelayGossipTo(dstPeerName, msg)
}

func (peer *LocalPeer) RelayGossipTo(dstPeerName PeerName, msg []byte) error {
	relayPeerName, found := peer.Router.Routes.Unicast(dstPeerName)
	if !found {
		log.Println("Cannot relay gossip for unknown destination:", dstPeerName)
		return nil
	}
	conn, found := peer.ConnectionTo(relayPeerName)
	if !found {
		log.Println("Gossip: Unable to find connection to relay peer", relayPeerName)
		return nil
	}
	conn.(*LocalConnection).SendTCP(msg)
	return nil
}

func (ourself *LocalPeer) OnAlive(peer *Peer) {
	for _, c := range ourself.Router.GossipChannels {
		c.gossiper.OnAlive(peer.UID)
	}
}

func (ourself *LocalPeer) OnDead(peer *Peer) {
	for _, c := range ourself.Router.GossipChannels {
		c.gossiper.OnDead(peer.UID)
	}
}
