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
	macs    *MacCache
	table   map[PeerName]*Peer
	onGC    func(*Peer)
}

func NewPeers(ourself *Peer, macs *MacCache, onGC func(*Peer)) *Peers {
	return &Peers{
		ourself: ourself,
		macs:    macs,
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

func (peers *Peers) ApplyUpdate(update []byte) ([]byte, error) {
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

	return encodePeersMap(newUpdate), nil
}

func (peers *Peers) EncodeAllPeers() []byte {
	peers.RLock()
	defer peers.RUnlock()
	return encodePeersMap(peers.table)
}

func EncodePeers(peers ...*Peer) []byte {
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	for _, peer := range peers {
		peer.encodePeer(enc)
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
		buf.WriteString(fmt.Sprint(peer, "\n"))
		peer.ForEachConnection(func(remoteName PeerName, conn Connection) {
			buf.WriteString(fmt.Sprintf("   -> %v [%v]\n", remoteName, conn.RemoteTCPAddr()))
		})
	})
	return buf.String()
}

func (peers *Peers) fetchAlias(peer *Peer) (*Peer, bool) {
	if existingPeer, found := peers.table[peer.Name]; found {
		if existingPeer.UID == peer.UID {
			existingPeer.IncrementLocalRefCount()
			return existingPeer, true
		} else {
			return nil, true
		}
	}
	return nil, false
}

func (peers *Peers) garbageCollect() []*Peer {
	removed := []*Peer{}
	for name, peer := range peers.table {
		found, _ := peers.ourself.Routes(peer, false)
		if !found && !peer.IsLocallyReferenced() {
			peers.onGC(peer)
			delete(peers.table, name)
			peers.macs.Delete(peer)
			removed = append(removed, peer)
		}
	}
	return removed
}

func encodePeersMap(peers map[PeerName]*Peer) []byte {
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	for _, peer := range peers {
		peer.encodePeer(enc)
	}
	return buf.Bytes()
}

func (peers *Peers) decodeUpdate(update []byte) (newPeers map[PeerName]*Peer, decodedUpdate []*Peer, decodedConns [][]byte, err error) {
	newPeers = make(map[PeerName]*Peer)
	decodedUpdate = []*Peer{}
	decodedConns = [][]byte{}

	updateBuf := new(bytes.Buffer)
	updateBuf.Write(update)
	decoder := gob.NewDecoder(updateBuf)

	for {
		nameByte, uid, version, connsBuf, decErr := decodePeerNoConns(decoder)
		if decErr == io.EOF {
			break
		} else if decErr != nil {
			err = decErr
			return
		}
		name := PeerNameFromBin(nameByte)
		newPeer := NewPeer(name, uid, version)
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

	unknownPeers := false
	for _, connsBuf := range decodedConns {
		connsIterator(connsBuf, func(remoteNameByte []byte, _ string) {
			remoteName := PeerNameFromBin(remoteNameByte)
			if _, found := newPeers[remoteName]; found {
				return
			}
			if _, found := peers.table[remoteName]; found {
				return
			}
			// Update refers to a peer which we have no knowledge
			// of. Thus we can't apply the update. Abort.
			unknownPeers = true
		})
	}
	if unknownPeers {
		err = UnknownPeersError{}
		return
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
		if peer != newPeer {
			if peer.Version() > newPeer.Version() {
				// we know more about this one than the update. If
				// peer is ourself, this is slightly racey (further
				// changes could occur to ourself in the mean
				// time). But it doesn't matter as we know that we're
				// already > newPeer.Version() and that's not going to
				// change.
				newUpdate[name] = peer
				continue
			} else if peer == peers.ourself {
				// nobody but us updates us
				continue
			} else if peer.Version() == newPeer.Version() {
				// implication is that connections are equal too
				continue
			}
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

func (peer *Peer) encodePeer(enc *gob.Encoder) {
	peer.RLock()
	defer peer.RUnlock()

	checkFatal(enc.Encode(peer.NameByte))
	checkFatal(enc.Encode(peer.UID))
	checkFatal(enc.Encode(peer.version))

	connsBuf := new(bytes.Buffer)
	connsEnc := gob.NewEncoder(connsBuf)
	for _, conn := range peer.connections {
		// DANGER holding rlock on peer, going to take rlock on conn
		if !conn.Established() {
			continue
		}
		checkFatal(connsEnc.Encode(conn.Remote().NameByte))
		checkFatal(connsEnc.Encode(conn.RemoteTCPAddr()))
	}
	checkFatal(enc.Encode(connsBuf.Bytes()))
}

func decodePeerNoConns(dec *gob.Decoder) (nameByte []byte, uid uint64, version uint64, conns []byte, err error) {
	err = dec.Decode(&nameByte)
	if err != nil {
		return
	}
	err = dec.Decode(&uid)
	if err != nil {
		return
	}
	err = dec.Decode(&version)
	if err != nil {
		return
	}
	err = dec.Decode(&conns)
	if err == io.EOF {
		err = nil
	}
	return
}

func connsIterator(input []byte, fun func([]byte, string)) {
	buf := new(bytes.Buffer)
	buf.Write(input)
	dec := gob.NewDecoder(buf)
	for {
		var nameByte []byte
		err := dec.Decode(&nameByte)
		if err == io.EOF {
			return
		}
		checkFatal(err)
		var foundAt string
		checkFatal(dec.Decode(&foundAt))
		fun(nameByte, string(foundAt))
	}
}

func readConnsMap(peer *Peer, buf []byte, table map[PeerName]*Peer) map[PeerName]Connection {
	conns := make(map[PeerName]Connection)
	connsIterator(buf, func(nameByte []byte, remoteTCPAddr string) {
		name := PeerNameFromBin(nameByte)
		remotePeer := table[name]
		conn := NewRemoteConnection(peer, remotePeer, remoteTCPAddr)
		conns[name] = conn
	})
	return conns
}
