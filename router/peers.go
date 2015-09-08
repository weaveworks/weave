package router

import (
	"bytes"
	"encoding/gob"
	"io"
	"sync"
)

type Peers struct {
	sync.RWMutex
	ourself *LocalPeer
	byName  map[PeerName]*Peer
	onGC    []func(*Peer)
}

type UnknownPeerError struct {
	Name PeerName
}

type NameCollisionError struct {
	Name PeerName
}

type PeerNameSet map[PeerName]struct{}

type ConnectionSummary struct {
	NameByte      []byte
	RemoteTCPAddr string
	Outbound      bool
	Established   bool
}

// Pending notifications due to changes to Peers that need to be ent
// out once the Peers is unlocked.
type PeersPendingNotifications struct {
	// Peers that have been GCed
	removed []*Peer
}

func NewPeers(ourself *LocalPeer) *Peers {
	peers := &Peers{ourself: ourself, byName: make(map[PeerName]*Peer)}
	peers.FetchWithDefault(ourself.Peer)
	return peers
}

func (peers *Peers) OnGC(callback func(*Peer)) {
	peers.Lock()
	defer peers.Unlock()

	// Although the array underlying peers.onGC might be accessed
	// without holding the lock in unlockAndNotify, we don't
	// support removing callbacks, so a simple append here is
	// safe.
	peers.onGC = append(peers.onGC, callback)
}

func (peers *Peers) unlockAndNotify(pending *PeersPendingNotifications) {
	onGC := peers.onGC
	peers.Unlock()
	if pending.removed != nil {
		for _, callback := range onGC {
			for _, peer := range pending.removed {
				callback(peer)
			}
		}
	}
}

func (peers *Peers) FetchWithDefault(peer *Peer) *Peer {
	peers.Lock()
	defer peers.Unlock()
	if existingPeer, found := peers.byName[peer.Name]; found {
		existingPeer.localRefCount++
		return existingPeer
	}
	peers.byName[peer.Name] = peer
	peer.localRefCount++
	return peer
}

func (peers *Peers) Fetch(name PeerName) *Peer {
	peers.RLock()
	defer peers.RUnlock()
	return peers.byName[name]
}

func (peers *Peers) FetchAndAddRef(name PeerName) *Peer {
	peers.Lock()
	defer peers.Unlock()
	peer := peers.byName[name]
	if peer != nil {
		peer.localRefCount++
	}
	return peer
}

func (peers *Peers) Dereference(peer *Peer) {
	peers.Lock()
	defer peers.Unlock()
	peer.localRefCount--
}

func (peers *Peers) ForEach(fun func(*Peer)) {
	peers.RLock()
	defer peers.RUnlock()
	for _, peer := range peers.byName {
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
	var pending PeersPendingNotifications
	defer peers.unlockAndNotify(&pending)

	newPeers, decodedUpdate, decodedConns, err := peers.decodeUpdate(update)
	if err != nil {
		return nil, nil, err
	}

	// By this point, we know the update doesn't refer to any peers we
	// have no knowledge of. We can now apply the update. Start by
	// adding in any new peers into the cache.
	for name, newPeer := range newPeers {
		peers.byName[name] = newPeer
	}

	// Now apply the updates
	newUpdate := peers.applyUpdate(decodedUpdate, decodedConns)
	peers.garbageCollect(&pending)
	for _, peerRemoved := range pending.removed {
		delete(newUpdate, peerRemoved.Name)
	}

	updateNames := make(PeerNameSet)
	for _, peer := range decodedUpdate {
		updateNames[peer.Name] = void
	}

	return updateNames, newUpdate, nil
}

func (peers *Peers) Names() PeerNameSet {
	peers.RLock()
	defer peers.RUnlock()

	names := make(PeerNameSet)
	for name := range peers.byName {
		names[name] = void
	}
	return names
}

func (peers *Peers) EncodePeers(names PeerNameSet) []byte {
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	peers.RLock()
	defer peers.RUnlock()
	for name := range names {
		if peer, found := peers.byName[name]; found {
			if peer == peers.ourself.Peer {
				peers.ourself.Encode(enc)
			} else {
				peer.Encode(enc)
			}
		}
	}
	return buf.Bytes()
}

func (peers *Peers) GarbageCollect() {
	peers.Lock()
	var pending PeersPendingNotifications
	defer peers.unlockAndNotify(&pending)

	peers.garbageCollect(&pending)
}

func (peers *Peers) garbageCollect(pending *PeersPendingNotifications) {
	peers.ourself.RLock()
	_, reached := peers.ourself.Routes(nil, false)
	peers.ourself.RUnlock()
	for name, peer := range peers.byName {
		if _, found := reached[peer.Name]; !found && peer.localRefCount == 0 {
			delete(peers.byName, name)
			pending.removed = append(pending.removed, peer)
		}
	}
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
		newPeer := NewPeerFromSummary(peerSummary)
		decodedUpdate = append(decodedUpdate, newPeer)
		decodedConns = append(decodedConns, connSummaries)
		existingPeer, found := peers.byName[newPeer.Name]
		if !found {
			newPeers[newPeer.Name] = newPeer
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
			if _, found := peers.byName[remoteName]; found {
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

func (peers *Peers) applyUpdate(decodedUpdate []*Peer, decodedConns [][]ConnectionSummary) PeerNameSet {
	newUpdate := make(PeerNameSet)
	for idx, newPeer := range decodedUpdate {
		connSummaries := decodedConns[idx]
		name := newPeer.Name
		// guaranteed to find peer in the peers.byName
		peer := peers.byName[name]
		if peer != newPeer &&
			(peer == peers.ourself.Peer || peer.Version >= newPeer.Version) {
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
		peer.Version = newPeer.Version
		peer.connections = makeConnsMap(peer, connSummaries, peers.byName)
		newUpdate[name] = void
	}
	return newUpdate
}

func (peer *Peer) Encode(enc *gob.Encoder) {
	checkFatal(enc.Encode(peer.PeerSummary))

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

func makeConnsMap(peer *Peer, connSummaries []ConnectionSummary, byName map[PeerName]*Peer) map[PeerName]Connection {
	conns := make(map[PeerName]Connection)
	for _, connSummary := range connSummaries {
		name := PeerNameFromBin(connSummary.NameByte)
		remotePeer := byName[name]
		conn := NewRemoteConnection(peer, remotePeer, connSummary.RemoteTCPAddr, connSummary.Outbound, connSummary.Established)
		conns[name] = conn
	}
	return conns
}
