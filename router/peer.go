package router

import (
	"fmt"
	"sort"
	"sync"
)

type Peer struct {
	sync.RWMutex
	Name          PeerName
	NameByte      []byte
	NickName      string
	UID           uint64
	version       uint64
	localRefCount uint64 // maintained by Peers
	connections   map[PeerName]Connection
}

type ConnectionSet map[Connection]struct{}

func NewPeer(name PeerName, nickName string, uid uint64, version uint64) *Peer {
	if uid == 0 {
		uid = randUint64()
	}
	return &Peer{
		Name:        name,
		NameByte:    name.Bin(),
		NickName:    nickName,
		UID:         uid,
		version:     version,
		connections: make(map[PeerName]Connection)}
}

func (peer *Peer) String() string {
	return fmt.Sprint(peer.Name, "(", peer.NickName, ")")
}

func (peer *Peer) Info() string {
	return fmt.Sprint(peer.String(), " (v", peer.version, ") (UID ", peer.UID, ")")
}

// Calculate the routing table from this peer to all peers reachable
// from it, returning a "next hop" map of PeerNameX -> PeerNameY,
// which says "in order to send a message to X, the peer should send
// the message to its neighbour Y".
//
// Because currently we do not have weightings on the connections
// between peers, there is no need to use a minimum spanning tree
// algorithm. Instead we employ the simpler and cheaper breadth-first
// widening. The computation is deterministic, which ensures that when
// it is performed on the same data by different peers, they get the
// same result. This is important since otherwise we risk message loss
// or routing cycles.
//
// When the 'establishedAndSymmetric' flag is set, only connections
// that are marked as 'established' and are symmetric (i.e. where both
// sides indicate they have a connection to the other) are considered.
//
// When a non-nil stopAt peer is supplied, the widening stops when it
// reaches that peer. The boolean return indicates whether that has
// happened.
//
// We acquire read locks on peers as we encounter them during the
// traversal. This prevents the connectivity graph from changing
// underneath us in ways that would invalidate the result. Thus the
// answer returned may be out of date, but never inconsistent.
func (peer *Peer) Routes(stopAt *Peer, establishedAndSymmetric bool) (bool, map[PeerName]PeerName) {
	peer.RLock()
	defer peer.RUnlock()
	routes := make(map[PeerName]PeerName)
	routes[peer.Name] = UnknownPeerName
	nextWorklist := []*Peer{peer}
	for len(nextWorklist) > 0 {
		worklist := nextWorklist
		sort.Sort(ListOfPeers(worklist))
		nextWorklist = []*Peer{}
		for _, curPeer := range worklist {
			if curPeer == stopAt {
				return true, routes
			}
			curName := curPeer.Name
			for remoteName, conn := range curPeer.connections {
				if establishedAndSymmetric && !conn.Established() {
					continue
				}
				if _, found := routes[remoteName]; found {
					continue
				}
				remote := conn.Remote()
				remote.RLock()
				// DANGER holding rlock on remote, possibly taking
				// rlock on remoteConn
				if remoteConn, found := remote.connections[curName]; !establishedAndSymmetric || (found && remoteConn.Established()) {
					defer remote.RUnlock()
					nextWorklist = append(nextWorklist, remote)
					// We now know how to get to remoteName: the same
					// way we get to curPeer. Except, if curPeer is
					// the starting peer in which case we know we can
					// reach remoteName directly.
					if curPeer == peer {
						routes[remoteName] = remoteName
					} else {
						routes[remoteName] = routes[curName]
					}
				} else {
					remote.RUnlock()
				}
			}
		}
	}
	return false, routes
}
