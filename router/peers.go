package router

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
	"sync"
)

type Peers struct {
	sync.RWMutex
	ourself *LocalPeer
	table   map[PeerName]*Peer
	onGC    func(*Peer)
}

type UnknownPeerError struct {
	Name PeerName
}

type NameCollisionError struct {
	Name PeerName
}

type PeerNameSet map[PeerName]struct{}

type PeerSummary struct {
	NameByte []byte
	NickName string
	UID      PeerUID
	Version  uint64
}

type ConnectionSummary struct {
	NameByte      []byte
	RemoteTCPAddr string
	Outbound      bool
	Established   bool
}

func NewPeers(ourself *LocalPeer, onGC func(*Peer)) *Peers {
	return &Peers{
		ourself: ourself,
		table:   make(map[PeerName]*Peer),
		onGC:    onGC}
}

func (peers *Peers) FetchWithDefault(peer *Peer) *Peer {
	peers.Lock()
	defer peers.Unlock()
	if existingPeer, found := peers.table[peer.Name]; found {
		if existingPeer.UID != peer.UID {
			return nil
		}
		existingPeer.localRefCount++
		return existingPeer
	}
	peers.table[peer.Name] = peer
	peer.localRefCount++
	return peer
}

func (peers *Peers) Fetch(name PeerName) *Peer {
	peers.RLock()
	defer peers.RUnlock()
	return peers.table[name]
}

func (peers *Peers) Dereference(peer *Peer) {
	peers.Lock()
	defer peers.Unlock()
	peer.localRefCount--
}

func (peers *Peers) ForEach(fun func(*Peer)) {
	peers.RLock()
	defer peers.RUnlock()
	for _, peer := range peers.table {
		fun(peer)
	}
}

// Merge an incoming update with our own topology.
//
// We add peers hitherto unknown to us, and update peers for which the
// update contains a more recent version than known to us. The return
// value is a) a representation of the received update, and b) an
// "improved" update containing just these new/updated elements.
func (peers *Peers) ApplyUpdate(update []byte) (PeerNameSet, PeerNameSet, error) {
	peers.Lock()

	newPeers, decodedUpdate, decodedConns, err := peers.decodeUpdate(update)
	if err != nil {
		peers.Unlock()
		return nil, nil, err
	}

	// By this point, we know the update doesn't refer to any peers we
	// have no knowledge of. We can now apply the update. Start by
	// adding in any new peers into the cache.
	for name, newPeer := range newPeers {
		peers.table[name] = newPeer
	}

	// Now apply the updates
	newUpdate := peers.applyUpdate(decodedUpdate, decodedConns)

	for _, peerRemoved := range peers.garbageCollect() {
		delete(newUpdate, peerRemoved.Name)
	}

	// Don't need to hold peers lock any longer
	peers.Unlock()

	updateNames := make(PeerNameSet)
	for _, peer := range decodedUpdate {
		updateNames[peer.Name] = void
	}
	return updateNames, setFromPeersMap(newUpdate), nil
}

func (peers *Peers) Names() PeerNameSet {
	peers.RLock()
	defer peers.RUnlock()
	return setFromPeersMap(peers.table)
}

func (peers *Peers) EncodePeers(names PeerNameSet) []byte {
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	peers.RLock()
	defer peers.RUnlock()
	for name := range names {
		if peer, found := peers.table[name]; found {
			if peer == peers.ourself.Peer {
				peers.ourself.Encode(enc)
			} else {
				peer.Encode(enc)
			}
		}
	}
	return buf.Bytes()
}

func (peers *Peers) GarbageCollect() []*Peer {
	peers.Lock()
	defer peers.Unlock()
	return peers.garbageCollect()
}

func (peers *Peers) String() string {
	var buf bytes.Buffer
	printConnection := func(conn Connection) {
		established := ""
		if !conn.Established() {
			established = " (unestablished)"
		}
		fmt.Fprintf(&buf, "   -> %s [%v%s]\n", conn.Remote(), conn.RemoteTCPAddr(), established)
	}
	peers.ForEach(func(peer *Peer) {
		if peer == peers.ourself.Peer {
			fmt.Fprintln(&buf, peers.ourself.Info())
			for conn := range peers.ourself.Connections() {
				printConnection(conn)
			}
		} else {
			fmt.Fprintln(&buf, peer.Info())
			// Modifying peer.connections requires a write lock on
			// Peers, and since we are holding a read lock (due to the
			// ForEach), access without locking the peer is safe.
			for _, conn := range peer.connections {
				printConnection(conn)
			}
		}
	})
	return buf.String()
}

func (peers *Peers) garbageCollect() []*Peer {
	removed := []*Peer{}
	peers.ourself.RLock()
	_, reached := peers.ourself.Routes(nil, false)
	peers.ourself.RUnlock()
	for name, peer := range peers.table {
		if _, found := reached[peer.Name]; !found && peer.localRefCount == 0 {
			delete(peers.table, name)
			peers.onGC(peer)
			removed = append(removed, peer)
		}
	}
	return removed
}

func setFromPeersMap(peers map[PeerName]*Peer) PeerNameSet {
	names := make(PeerNameSet)
	for name := range peers {
		names[name] = void
	}
	return names
}

func (peers *Peers) decodeUpdate(update []byte) (newPeers map[PeerName]*Peer, decodedUpdate []*Peer, decodedConns [][]ConnectionSummary, err error) {
	newPeers = make(map[PeerName]*Peer)
	decodedUpdate = []*Peer{}
	decodedConns = [][]ConnectionSummary{}

	decoder := gob.NewDecoder(bytes.NewReader(update))

	for {
		peerSummary, connSummaries, decErr := decodePeer(decoder)
		if decErr == io.EOF {
			break
		} else if decErr != nil {
			err = decErr
			return
		}
		name := PeerNameFromBin(peerSummary.NameByte)
		newPeer := NewPeer(name, peerSummary.NickName, peerSummary.UID, peerSummary.Version)
		decodedUpdate = append(decodedUpdate, newPeer)
		decodedConns = append(decodedConns, connSummaries)
		existingPeer, found := peers.table[name]
		if !found {
			newPeers[name] = newPeer
		} else if existingPeer.UID != newPeer.UID {
			err = NameCollisionError{Name: newPeer.Name}
			return
		}
	}

	for _, connSummaries := range decodedConns {
		for _, connSummary := range connSummaries {
			remoteName := PeerNameFromBin(connSummary.NameByte)
			if _, found := newPeers[remoteName]; found {
				continue
			}
			if _, found := peers.table[remoteName]; found {
				continue
			}
			// Update refers to a peer which we have no knowledge
			// of. Thus we can't apply the update. Abort.
			err = UnknownPeerError{remoteName}
			return
		}
	}
	return
}

func (peers *Peers) applyUpdate(decodedUpdate []*Peer, decodedConns [][]ConnectionSummary) map[PeerName]*Peer {
	newUpdate := make(map[PeerName]*Peer)
	for idx, newPeer := range decodedUpdate {
		connSummaries := decodedConns[idx]
		name := newPeer.Name
		// guaranteed to find peer in the peers.table
		peer := peers.table[name]
		if peer != newPeer &&
			(peer == peers.ourself.Peer || peer.version >= newPeer.version) {
			// Nobody but us updates us. And if we know more about a
			// peer than what's in the the update, we ignore the
			// latter.
			continue
		}
		// If we're here, either it was a new peer, or the update has
		// more info about the peer than we do. Either case, we need
		// to set version and conns and include the updated peer in
		// the outgoing update.

		// Can peer have been updated by anyone else in the mean time?
		// No - we know that peer is not ourself, so the only prospect
		// for an update would be someone else calling
		// router.Peers.ApplyUpdate. But ApplyUpdate takes the Lock on
		// the router.Peers, so there can be no race here.
		peer.version = newPeer.version
		peer.connections = makeConnsMap(peer, connSummaries, peers.table)
		newUpdate[name] = peer
	}
	return newUpdate
}

func (peer *Peer) Encode(enc *gob.Encoder) {
	checkFatal(enc.Encode(PeerSummary{
		peer.NameByte,
		peer.NickName,
		peer.UID,
		peer.version}))

	connSummaries := []ConnectionSummary{}
	for _, conn := range peer.connections {
		connSummaries = append(connSummaries, ConnectionSummary{
			conn.Remote().NameByte,
			conn.RemoteTCPAddr(),
			conn.Outbound(),
			conn.Established(),
		})
	}

	checkFatal(enc.Encode(connSummaries))
}

func (peer *LocalPeer) Encode(enc *gob.Encoder) {
	peer.RLock()
	defer peer.RUnlock()
	peer.Peer.Encode(enc)
}

func decodePeer(dec *gob.Decoder) (peerSummary PeerSummary, connSummaries []ConnectionSummary, err error) {
	if err = dec.Decode(&peerSummary); err != nil {
		return
	}
	if err = dec.Decode(&connSummaries); err != nil {
		return
	}
	return
}

func makeConnsMap(peer *Peer, connSummaries []ConnectionSummary, table map[PeerName]*Peer) map[PeerName]Connection {
	conns := make(map[PeerName]Connection)
	for _, connSummary := range connSummaries {
		name := PeerNameFromBin(connSummary.NameByte)
		remotePeer := table[name]
		conn := NewRemoteConnection(peer, remotePeer, connSummary.RemoteTCPAddr, connSummary.Outbound, connSummary.Established)
		conns[name] = conn
	}
	return conns
}
