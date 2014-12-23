package router

import (
	"hash/fnv"
	"log"
)

func hash(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
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

func (peer *Peer) RelayGossipBroadcast(srcName PeerName, msg []byte) {
	if srcPeer, found := peer.Router.Peers.Fetch(srcName); found {
		peer.CallBroadcastFunc(srcPeer, func(conn *LocalConnection) error {
			conn.SendTCP(msg)
			return nil
		})
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
	return c.localPeer.RelayGossipTo(c.localPeer.Name, dstPeerName, msg)
}

func (peer *Peer) RelayGossipTo(srcPeerName, dstPeerName PeerName, msg []byte) error {
	relayPeerName, found := peer.Router.Topology.Unicast(dstPeerName)
	if !found {
		peer.Router.Topology.RebuildRoutes()
		peer.Router.Topology.Sync()
		relayPeerName, found = peer.Router.Topology.Unicast(dstPeerName)
		if !found {
			log.Println("Cannot relay gossip for unknown destination:", dstPeerName)
			return nil
		}
	}
	conn, found := peer.ConnectionTo(relayPeerName)
	if !found {
		log.Println("Gossip: Unable to find connection to relay peer", relayPeerName)
		return nil
	}
	conn.(*LocalConnection).SendTCP(msg)
	return nil
}

func (ourself *Peer) OnDead(peer *Peer) {
	for _, c := range ourself.Router.GossipChannels {
		c.gossiper.OnDead(peer.UID)
	}
}
