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
	ourself *Peer
	table   map[PeerName]*Peer
	onGC    func(*Peer)
}

type UnknownPeerError struct {
	Name PeerName
}

type NameCollisionError struct {
	Name PeerName
}

func NewPeers(ourself *Peer, onGC func(*Peer)) *Peers {
	return &Peers{
		ourself: ourself,
		table:   make(map[PeerName]*Peer),
		onGC:    onGC}
}

func (peers *Peers) FetchWithDefault(peer *Peer) *Peer {
	peers.RLock()
	res, found := peers.fetchAlias(peer)
	peers.RUnlock()
	if found {
		return res
	}
	peers.Lock()
	defer peers.Unlock()
	res, found = peers.fetchAlias(peer)
	if found {
		return res
	}
	peers.table[peer.Name] = peer
	peer.IncrementLocalRefCount()
	return peer
}

func (peers *Peers) Fetch(name PeerName) (*Peer, bool) {
	peers.RLock()
	defer peers.RUnlock()
	peer, found := peers.table[name]
	return peer, found // GRRR, why can't I inline this!?
}

func (peers *Peers) ForEach(fun func(PeerName, *Peer)) {
	peers.RLock()
	defer peers.RUnlock()
	for name, peer := range peers.table {
		fun(name, peer)
	}
}

// Merge an incoming update with our own topology.
//
// We add peers hitherto unknown to us, and update peers for which the
// update contains a more recent version than known to us. The return
// value is an "improved" update containing just these new/updated
// elements.
func (peers *Peers) ApplyUpdate(update []byte) (PeerNameSet, error) {
	peers.Lock()

	newPeers, decodedUpdate, decodedConns, err := peers.decodeUpdate(update)
	if err != nil {
		peers.Unlock()
		return nil, err
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

	return setFromPeersMap(newUpdate), nil
}

func (peers *Peers) Names() PeerNameSet {
	peers.RLock()
	defer peers.RUnlock()
	return setFromPeersMap(peers.table)
}

func (peers *Peers) EncodePeers(names PeerNameSet) []byte {
	peers.RLock()
	peerList := make([]*Peer, 0, len(names))
	for name := range names {
		if peer, found := peers.table[name]; found {
			peerList = append(peerList, peer)
		}
	}
	peers.RUnlock() // release lock so we don't hold it while encoding
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	for _, peer := range peerList {
		peer.encode(enc)
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
	peers.ForEach(func(name PeerName, peer *Peer) {
		fmt.Fprint(&buf, peer, "\n")
		for _, conn := range peer.Connections() {
			established := ""
			if !conn.Established() {
				established = " (unestablished)"
			}
			fmt.Fprintf(&buf, "   -> %v (%v) [%v%s]\n", conn.Remote().Name, conn.Remote().NickName, conn.RemoteTCPAddr(), established)
		}
	})
	return buf.String()
}

func (peers *Peers) fetchAlias(peer *Peer) (*Peer, bool) {
	if existingPeer, found := peers.table[peer.Name]; found {
		if existingPeer.UID != peer.UID {
			return nil, true
		}
		existingPeer.IncrementLocalRefCount()
		return existingPeer, true
	}
	return nil, false
}

func (peers *Peers) garbageCollect() []*Peer {
	removed := []*Peer{}
	_, reached := peers.ourself.Routes(nil, false)
	for name, peer := range peers.table {
		if _, found := reached[peer.Name]; !found && !peer.IsLocallyReferenced() {
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
		names[name] = true
	}
	return names
}

func (peers *Peers) decodeUpdate(update []byte) (newPeers map[PeerName]*Peer, decodedUpdate []*Peer, decodedConns [][]byte, err error) {
	newPeers = make(map[PeerName]*Peer)
	decodedUpdate = []*Peer{}
	decodedConns = [][]byte{}

	updateBuf := new(bytes.Buffer)
	updateBuf.Write(update)
	decoder := gob.NewDecoder(updateBuf)

	for {
		nameByte, nickName, uid, version, connsBuf, decErr := decodePeerNoConns(decoder)
		if decErr == io.EOF {
			break
		} else if decErr != nil {
			err = decErr
			return
		}
		name := PeerNameFromBin(nameByte)
		newPeer := NewPeer(name, nickName, uid, version)
		decodedUpdate = append(decodedUpdate, newPeer)
		decodedConns = append(decodedConns, connsBuf)
		existingPeer, found := peers.table[name]
		if !found {
			newPeers[name] = newPeer
		} else if existingPeer.UID != newPeer.UID {
			err = NameCollisionError{Name: newPeer.Name}
			return
		}
	}

	for _, connsBuf := range decodedConns {
		decErr := connsIterator(connsBuf, func(remoteNameByte []byte, _ string, _, _ bool) {
			remoteName := PeerNameFromBin(remoteNameByte)
			if _, found := newPeers[remoteName]; found {
				return
			}
			if _, found := peers.table[remoteName]; found {
				return
			}
			// Update refers to a peer which we have no knowledge
			// of. Thus we can't apply the update. Abort.
			err = UnknownPeerError{remoteName}
		})
		if decErr != nil && decErr != io.EOF {
			err = decErr
		}
		if err != nil {
			return
		}
	}
	return
}

func (peers *Peers) applyUpdate(decodedUpdate []*Peer, decodedConns [][]byte) map[PeerName]*Peer {
	newUpdate := make(map[PeerName]*Peer)
	for idx, newPeer := range decodedUpdate {
		connsBuf := decodedConns[idx]
		name := newPeer.Name
		// guaranteed to find peer in the peers.table
		peer := peers.table[name]
		if peer != newPeer &&
			(peer == peers.ourself || peer.Version() >= newPeer.Version()) {
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
		conns := readConnsMap(peer, connsBuf, peers.table)
		peer.SetVersionAndConnections(newPeer.Version(), conns)
		newUpdate[name] = peer
	}
	return newUpdate
}

func (peer *Peer) encode(enc *gob.Encoder) {
	peer.RLock()
	defer peer.RUnlock()

	checkFatal(enc.Encode(peer.NameByte))
	checkFatal(enc.Encode(peer.NickName))
	checkFatal(enc.Encode(peer.UID))
	checkFatal(enc.Encode(peer.version))

	connsBuf := new(bytes.Buffer)
	connsEnc := gob.NewEncoder(connsBuf)
	for _, conn := range peer.connections {
		checkFatal(connsEnc.Encode(conn.Remote().NameByte))
		checkFatal(connsEnc.Encode(conn.RemoteTCPAddr()))
		checkFatal(connsEnc.Encode(conn.Outbound()))
		// DANGER holding rlock on peer, going to take rlock on conn
		checkFatal(connsEnc.Encode(conn.Established()))
	}
	checkFatal(enc.Encode(connsBuf.Bytes()))
}

func decodePeerNoConns(dec *gob.Decoder) (nameByte []byte, nickName string, uid uint64, version uint64, conns []byte, err error) {
	if err = dec.Decode(&nameByte); err != nil {
		return
	}
	if err = dec.Decode(&nickName); err != nil {
		return
	}
	if err = dec.Decode(&uid); err != nil {
		return
	}
	if err = dec.Decode(&version); err != nil {
		return
	}
	if err = dec.Decode(&conns); err != nil {
		return
	}
	return
}

func connsIterator(input []byte, fun func([]byte, string, bool, bool)) error {
	buf := new(bytes.Buffer)
	buf.Write(input)
	dec := gob.NewDecoder(buf)
	for {
		var nameByte []byte
		if err := dec.Decode(&nameByte); err != nil {
			return err
		}
		var foundAt string
		if err := dec.Decode(&foundAt); err != nil {
			return err
		}
		var outbound bool
		if err := dec.Decode(&outbound); err != nil {
			return err
		}
		var established bool
		if err := dec.Decode(&established); err != nil {
			return err
		}
		fun(nameByte, foundAt, outbound, established)
	}
}

func readConnsMap(peer *Peer, buf []byte, table map[PeerName]*Peer) map[PeerName]Connection {
	conns := make(map[PeerName]Connection)
	if err := connsIterator(buf, func(nameByte []byte, remoteTCPAddr string, outbound bool, established bool) {
		name := PeerNameFromBin(nameByte)
		remotePeer := table[name]
		conn := NewRemoteConnection(peer, remotePeer, remoteTCPAddr, outbound, established)
		conns[name] = conn
	}); err != io.EOF {
		// this should never happen since we've already successfully
		// decoded the same data in decodeUpdate
		checkFatal(err)
	}
	return conns
}
