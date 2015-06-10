package router

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
	"math/rand"
	"sync"
)

type Peers struct {
	sync.RWMutex
	ourself                   *LocalPeer
	onGC                      func(*Peer)
	byName                    map[PeerName]*Peer
	byShortID                 map[PeerShortID]ShortIDPeers
	invalidateShortIDsPending bool
}

type ShortIDPeers struct {
	// If we know about a single peer with the short id, this is
	// that peer.  If there is a coliision, this is the peer with
	// the lowest Name.
	peer *Peer

	// In case of a collision, this holds the other ppeers.
	others []*Peer
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

func NewPeers(ourself *LocalPeer, onGC func(*Peer)) *Peers {
	peers := &Peers{
		ourself:   ourself,
		byName:    make(map[PeerName]*Peer),
		byShortID: make(map[PeerShortID]ShortIDPeers),
		onGC:      onGC}
	peers.FetchWithDefault(ourself.Peer)
	return peers
}

func (peers *Peers) Fetch(name PeerName) (*Peer, bool) {
	peers.RLock()
	defer peers.RUnlock()
	peer, found := peers.byName[name]
	return peer, found // GRRR, why can't I inline this!?
}

func (peers *Peers) FetchByShortID(shortID PeerShortID) *Peer {
	peers.RLock()
	defer peers.RUnlock()
	return peers.byShortID[shortID].peer
}

func (peers *Peers) addByShortID(peer *Peer) {
	reassign := false
	entry, ok := peers.byShortID[peer.ShortID]
	if !ok {
		entry = ShortIDPeers{peer: peer}
	} else if entry.peer == nil {
		// This short ID is free, but was used in the past.
		// Because we are reusing it, it's an invalidation
		// event.
		entry.peer = peer
		peers.invalidateShortIDsPending = true
	} else if peer.Name < entry.peer.Name {
		// Short ID collision, this peer becomes the principal
		// peer for the short ID, bumping the previous one
		// into others.

		if entry.peer == peers.ourself.Peer {
			// The bumped peer is peers.ourself, so we
			// need to look foor a new short id
			reassign = true
		}

		entry.others = append(entry.others, entry.peer)
		entry.peer = peer
		peers.invalidateShortIDsPending = true
	} else {
		// Short ID collision, this peer is secondary
		entry.others = append(entry.others, peer)
	}

	peers.byShortID[peer.ShortID] = entry

	if reassign {
		peers.reassignLocalShortID()
	}
}

func (peers *Peers) deleteByShortID(peer *Peer) {
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
		peers.invalidateShortIDsPending = true
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

func (peers *Peers) reassignLocalShortID() {
	newShortID, ok := peers.chooseShortID()
	if ok {
		peers.setLocalShortID(newShortID)
	}

	// Otherwise we'll try again in garbageColleect
}

func (peers *Peers) setLocalShortID(newShortID PeerShortID) {
	peers.deleteByShortID(peers.ourself.Peer)
	peers.ourself.setShortID(newShortID)
	peers.addByShortID(peers.ourself.Peer)
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
		if i == 0 {
			panic("chooseShortID broken")
		}
	}
}

func (peers *Peers) unlockAndMaybeInvalidateShortIDs() {
	pending := peers.invalidateShortIDsPending
	peers.invalidateShortIDsPending = false
	peers.Unlock()
	if pending {
		//  Some tests have a nil router
		router := peers.ourself.router
		if router != nil {
			router.InterHost.InvalidateShortIDs()
		}
	}
}

func (peers *Peers) FetchWithDefault(peer *Peer) *Peer {
	peers.Lock()
	defer peers.unlockAndMaybeInvalidateShortIDs()
	if existingPeer, found := peers.byName[peer.Name]; found {
		if existingPeer.UID != peer.UID {
			return nil
		}
		existingPeer.localRefCount++
		return existingPeer
	}

	peers.byName[peer.Name] = peer
	peers.addByShortID(peer)
	peer.localRefCount++
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
	peerList := make([]*Peer, 0, len(names))
	for name := range names {
		if peer, found := peers.byName[name]; found {
			if peer == peers.ourself.Peer {
				peers.ourself.Peer.Encode(enc)
			} else {
				peer.Encode(enc)
			}
			peerList = append(peerList, peer)
		}
	}
	return buf.Bytes()
}

func (peers *Peers) GarbageCollect() []*Peer {
	peers.Lock()
	defer peers.unlockAndMaybeInvalidateShortIDs()
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

	for name, peer := range peers.byName {
		if _, found := reached[peer.Name]; !found && peer.localRefCount == 0 {
			delete(peers.byName, name)
			peers.deleteByShortID(peer)
			peers.onGC(peer)
			removed = append(removed, peer)
		}
	}

	if peers.byShortID[peers.ourself.ShortID].peer != peers.ourself.Peer {
		// The local peer doesn't own its short id.  Garbage
		// collection might have freed some up, so try to reassign.
		peers.reassignLocalShortID()
	}

	return removed
}

// Merge an incoming update with our own topology.
//
// We add peers hitherto unknown to us, and update peers for which the
// update contains a more recent version than known to us. The return
// value is a) a representation of the received update, and b) an
// "improved" update containing just these new/updated elements.
func (peers *Peers) ApplyUpdate(update []byte) (PeerNameSet, PeerNameSet, error) {
	peers.Lock()
	defer peers.unlockAndMaybeInvalidateShortIDs()

	newPeers, decodedUpdate, decodedConns, err := peers.decodeUpdate(update)
	if err != nil {
		return nil, nil, err
	}

	// By this point, we know the update doesn't refer to any peers we
	// have no knowledge of. We can now apply the update. Start by
	// adding in any new peers into the cache.
	for name, newPeer := range newPeers {
		peers.byName[name] = newPeer
		peers.addByShortID(newPeer)
	}

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

		if newPeer.ShortID != peer.ShortID {
			peers.deleteByShortID(peer)
			peer.ShortID = newPeer.ShortID
			peers.addByShortID(peer)
		}

		newUpdate[name] = void
	}

	for _, peerRemoved := range peers.garbageCollect() {
		delete(newUpdate, peerRemoved.Name)
	}

	updateNames := make(PeerNameSet)
	for _, peer := range decodedUpdate {
		updateNames[peer.Name] = void
	}

	return updateNames, newUpdate, nil
}

func (peers *Peers) decodeUpdate(update []byte) (newPeers map[PeerName]*Peer, decodedUpdate []*Peer, decodedConns [][]ConnectionSummary, err error) {
	newPeers = make(map[PeerName]*Peer)
	decodedUpdate = []*Peer{}
	decodedConns = [][]ConnectionSummary{}

	updateBuf := new(bytes.Buffer)
	updateBuf.Write(update)
	decoder := gob.NewDecoder(updateBuf)

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

func (peer *Peer) Encode(enc *gob.Encoder) {
	checkFatal(enc.Encode(peer.PeerSummary))

	connSummaries := []ConnectionSummary{}
	for _, conn := range peer.connections {
		connSummaries = append(connSummaries, ConnectionSummary{
			conn.Remote().NameByte,
			conn.RemoteTCPAddr(),
			conn.Outbound(),
			// DANGER holding rlock on peer, going to take rlock on conn
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
