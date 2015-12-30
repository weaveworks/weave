package mesh

import (
	"bytes"
	"encoding/gob"
	"io"
	"math/rand"
	"sync"
)

var void = struct{}{}

type Peers struct {
	sync.RWMutex
	ourself   *LocalPeer
	byName    map[PeerName]*Peer
	byShortID map[PeerShortID]ShortIDPeers
	onGC      []func(*Peer)

	// Called when the mapping from short ids to peers changes
	onInvalidateShortIDs []func()
}

type ShortIDPeers struct {
	// If we know about a single peer with the short id, this is
	// that peer.  If there is a collision, this is the peer with
	// the lowest Name.
	peer *Peer

	// In case of a collision, this holds the other peers.
	others []*Peer
}

type PeerNameSet map[PeerName]struct{}

type ConnectionSummary struct {
	NameByte      []byte
	RemoteTCPAddr string
	Outbound      bool
	Established   bool
}

// Pending notifications due to changes to Peers that need to be sent
// out once the Peers is unlocked.
type PeersPendingNotifications struct {
	// Peers that have been GCed
	removed []*Peer

	// The mapping from shorts ids to peers changed
	invalidateShortIDs bool

	// The local short ID needs reassigning due to a collision
	reassignLocalShortID bool

	// The local peer was modified
	localPeerModified bool
}

func NewPeers(ourself *LocalPeer) *Peers {
	peers := &Peers{
		ourself:   ourself,
		byName:    make(map[PeerName]*Peer),
		byShortID: make(map[PeerShortID]ShortIDPeers),
	}
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

func (peers *Peers) OnInvalidateShortIDs(callback func()) {
	peers.Lock()
	defer peers.Unlock()

	// Safe, as in OnGC
	peers.onInvalidateShortIDs = append(peers.onInvalidateShortIDs, callback)
}

func (peers *Peers) unlockAndNotify(pending *PeersPendingNotifications) {
	broadcastLocalPeer := (pending.reassignLocalShortID && peers.reassignLocalShortID(pending)) ||
		pending.localPeerModified
	onGC := peers.onGC
	onInvalidateShortIDs := peers.onInvalidateShortIDs
	peers.Unlock()

	if pending.removed != nil {
		for _, callback := range onGC {
			for _, peer := range pending.removed {
				callback(peer)
			}
		}
	}

	if pending.invalidateShortIDs {
		for _, callback := range onInvalidateShortIDs {
			callback()
		}
	}

	if broadcastLocalPeer {
		peers.ourself.broadcastPeerUpdate()
	}
}

func (peers *Peers) addByShortID(peer *Peer, pending *PeersPendingNotifications) {
	if !peer.HasShortID {
		return
	}

	entry, ok := peers.byShortID[peer.ShortID]
	if !ok {
		entry = ShortIDPeers{peer: peer}
	} else if entry.peer == nil {
		// This short ID is free, but was used in the past.
		// Because we are reusing it, it's an invalidation
		// event.
		entry.peer = peer
		pending.invalidateShortIDs = true
	} else if peer.Name < entry.peer.Name {
		// Short ID collision, this peer becomes the principal
		// peer for the short ID, bumping the previous one
		// into others.

		if entry.peer == peers.ourself.Peer {
			// The bumped peer is peers.ourself, so we
			// need to look for a new short id
			pending.reassignLocalShortID = true
		}

		entry.others = append(entry.others, entry.peer)
		entry.peer = peer
		pending.invalidateShortIDs = true
	} else {
		// Short ID collision, this peer is secondary
		entry.others = append(entry.others, peer)
	}

	peers.byShortID[peer.ShortID] = entry
}

func (peers *Peers) deleteByShortID(peer *Peer, pending *PeersPendingNotifications) {
	if !peer.HasShortID {
		return
	}

	entry := peers.byShortID[peer.ShortID]
	var otherIndex int

	if peer != entry.peer {
		// peer is secondary, find its index in others
		otherIndex = -1

		for i, other := range entry.others {
			if peer == other {
				otherIndex = i
				break
			}
		}

		if otherIndex < 0 {
			return
		}
	} else if len(entry.others) != 0 {
		// need to find the peer with the lowest name to
		// become the new principal one
		otherIndex = 0
		minName := entry.others[0].Name

		for i := 1; i < len(entry.others); i++ {
			otherName := entry.others[i].Name
			if otherName < minName {
				minName = otherName
				otherIndex = i
			}
		}

		entry.peer = entry.others[otherIndex]
		pending.invalidateShortIDs = true
	} else {
		// This is the last peer with the short id.  We clear
		// the entry, don't delete it, in order to detect when
		// it gets re-used.
		peers.byShortID[peer.ShortID] = ShortIDPeers{}
		return
	}

	entry.others[otherIndex] = entry.others[len(entry.others)-1]
	entry.others = entry.others[:len(entry.others)-1]
	peers.byShortID[peer.ShortID] = entry
}

func (peers *Peers) reassignLocalShortID(pending *PeersPendingNotifications) bool {
	newShortID, ok := peers.chooseShortID()
	if ok {
		peers.setLocalShortID(newShortID, pending)
		return true
	}

	// Otherwise we'll try again later on in garbageColleect
	return false
}

func (peers *Peers) setLocalShortID(newShortID PeerShortID, pending *PeersPendingNotifications) {
	peers.deleteByShortID(peers.ourself.Peer, pending)
	peers.ourself.setShortID(newShortID)
	peers.addByShortID(peers.ourself.Peer, pending)
}

// Choose an available short id at random
func (peers *Peers) chooseShortID() (PeerShortID, bool) {
	rng := rand.New(rand.NewSource(int64(randUint64())))

	// First, just try picking some short ids at random, and
	// seeing if they are available:
	for i := 0; i < 10; i++ {
		shortID := PeerShortID(rng.Intn(1 << PeerShortIDBits))
		if peers.byShortID[shortID].peer == nil {
			return shortID, true
		}
	}

	// Looks like most short ids are used.  So count the number of
	// unused ones, and pick one at random.
	available := int(1 << PeerShortIDBits)
	for _, entry := range peers.byShortID {
		if entry.peer != nil {
			available--
		}
	}

	if available == 0 {
		// All short ids are used.
		return 0, false
	}

	n := rng.Intn(available)
	var i PeerShortID
	for {
		if peers.byShortID[i].peer == nil {
			if n == 0 {
				return PeerShortID(i), true
			}

			n--
		}

		i++
	}
}

func (peers *Peers) FetchWithDefault(peer *Peer) *Peer {
	peers.Lock()
	var pending PeersPendingNotifications
	defer peers.unlockAndNotify(&pending)

	if existingPeer, found := peers.byName[peer.Name]; found {
		existingPeer.localRefCount++
		return existingPeer
	}

	peers.byName[peer.Name] = peer
	peers.addByShortID(peer, &pending)
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

func (peers *Peers) FetchByShortID(shortID PeerShortID) *Peer {
	peers.RLock()
	defer peers.RUnlock()
	return peers.byShortID[shortID].peer
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

	// Add new peers
	for name, newPeer := range newPeers {
		peers.byName[name] = newPeer
		peers.addByShortID(newPeer, &pending)
	}

	// Now apply the updates
	newUpdate := peers.applyUpdate(decodedUpdate, decodedConns, &pending)
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
			peers.deleteByShortID(peer, pending)
			pending.removed = append(pending.removed, peer)
		}
	}

	if len(pending.removed) > 0 && peers.byShortID[peers.ourself.ShortID].peer != peers.ourself.Peer {
		// The local peer doesn't own its short id.  Garbage
		// collection might have freed some up, so try to
		// reassign.
		pending.reassignLocalShortID = true
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
		if _, found := peers.byName[newPeer.Name]; !found {
			newPeers[newPeer.Name] = newPeer
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
			// Update refers to a peer which we have no knowledge of.
			newPeers[remoteName] = NewPeerPlaceholder(remoteName)
		}
	}
	return
}

func (peers *Peers) applyUpdate(decodedUpdate []*Peer, decodedConns [][]ConnectionSummary, pending *PeersPendingNotifications) PeerNameSet {
	newUpdate := make(PeerNameSet)
	for idx, newPeer := range decodedUpdate {
		connSummaries := decodedConns[idx]
		name := newPeer.Name
		// guaranteed to find peer in the peers.byName
		switch peer := peers.byName[name]; peer {
		case peers.ourself.Peer:
			if newPeer.UID != peer.UID {
				// The update contains information about an old
				// incarnation of ourselves. We increase our version
				// number beyond that which we received, so our
				// information supersedes the old one when it is
				// received by other peers.
				pending.localPeerModified = peers.ourself.setVersionBeyond(newPeer.Version)
			}
		case newPeer:
			peer.connections = makeConnsMap(peer, connSummaries, peers.byName)
			newUpdate[name] = void
		default: // existing peer
			if newPeer.Version < peer.Version ||
				(newPeer.Version == peer.Version &&
					(newPeer.UID < peer.UID ||
						(newPeer.UID == peer.UID &&
							(!newPeer.HasShortID || peer.HasShortID)))) {
				continue
			}
			peer.Version = newPeer.Version
			peer.UID = newPeer.UID
			peer.NickName = newPeer.NickName
			peer.connections = makeConnsMap(peer, connSummaries, peers.byName)

			if newPeer.ShortID != peer.ShortID || newPeer.HasShortID != peer.HasShortID {
				peers.deleteByShortID(peer, pending)
				peer.ShortID = newPeer.ShortID
				peer.HasShortID = newPeer.HasShortID
				peers.addByShortID(peer, pending)
			}
			newUpdate[name] = void
		}
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
