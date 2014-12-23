package router

import (
	"log"
)

// contains state for everyone that sending peer knows
// done on an interval; sent by one peer down [all/random subset of] connections
// peers that receive it should examine the info, and if it is broadcast
func (peer *Peer) Gossip() {
	peer.GossipMsg(peer.Router.Gossiper.Gossip())
}

func (peer *Peer) GossipMsg(buf []byte) {
	peer.ForEachConnection(func(_ PeerName, conn Connection) {
		if conn.Established() {
			peer.gossipOn(conn.(*LocalConnection), buf)
		}
	})
}

func (peer *Peer) gossipOn(conn *LocalConnection, buf []byte) {
	peerName := peer.Name.Bin()
	nameLenByte := []byte{byte(len(peerName))}
	msg := Concat([]byte{ProtocolGossip}, nameLenByte, peerName, buf)
	conn.SendTCP(msg)
}

// intended for state from sending peer only
// done when there is a change that everyone should hear about quickly
// peers that receive it should relay it using broadcast topology.
func (peer *Peer) GossipBroadcast(buf []byte) error {
	peerName := peer.Name.Bin()
	nameLenByte := []byte{byte(len(peerName))}
	msg := Concat([]byte{ProtocolGossipBroadcast}, nameLenByte, peerName, buf)
	peer.RelayGossipBroadcast(peer.Name, msg)
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
func (peer *Peer) GossipUnicast(dstPeerName PeerName, buf []byte) error {
	srcPeerByte := peer.Name.Bin()
	nameLenByte := []byte{byte(len(srcPeerByte))}
	dstPeerByte := dstPeerName.Bin()
	dstNameLenByte := []byte{byte(len(dstPeerByte))}
	msg := Concat([]byte{ProtocolGossipUnicast}, nameLenByte, srcPeerByte, dstNameLenByte, dstPeerByte, buf)
	return peer.RelayGossipTo(peer.Name, dstPeerName, msg)
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
	ourself.Router.Gossiper.OnDead(peer.UID)
}
