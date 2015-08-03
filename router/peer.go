package router

import (
	"fmt"
	"sort"
	"strconv"
)

type PeerUID uint64

func randomPeerUID() PeerUID {
	return PeerUID(randUint64())
}

func ParsePeerUID(s string) (PeerUID, error) {
	uid, err := strconv.ParseUint(s, 10, 64)
	return PeerUID(uid), err
}

type PeerSummary struct {
	NameByte []byte
	NickName string
	UID      PeerUID
	Version  uint64
}

type Peer struct {
	Name PeerName
	PeerSummary
	localRefCount uint64 // maintained by Peers
	connections   map[PeerName]Connection
}

type ConnectionSet map[Connection]struct{}

func NewPeerFromSummary(summary PeerSummary) *Peer {
	return &Peer{
		Name:        PeerNameFromBin(summary.NameByte),
		PeerSummary: summary,
		connections: make(map[PeerName]Connection),
	}
}

func NewPeer(name PeerName, nickName string, uid PeerUID, version uint64) *Peer {
	return NewPeerFromSummary(PeerSummary{
		NameByte: name.Bin(),
		NickName: nickName,
		UID:      uid,
		Version:  version,
	})
}

func NewPeerFrom(peer *Peer) *Peer {
	return NewPeerFromSummary(peer.PeerSummary)
}

func (peer *Peer) String() string {
	return fmt.Sprint(peer.Name, "(", peer.NickName, ")")
}

func (peer *Peer) Info() string {
	return fmt.Sprint(peer.String(), " (v", peer.Version, ") (UID ", peer.UID, ")")
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
// NB: This function should generally be invoked while holding a read
// lock on Peers and LocalPeer.
func (peer *Peer) Routes(stopAt *Peer, establishedAndSymmetric bool) (bool, map[PeerName]PeerName) {
	routes := make(unicastRoutes)
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
			curPeer.ForEachConnectedPeer(establishedAndSymmetric, routes,
				func(remotePeer *Peer) {
					nextWorklist = append(nextWorklist, remotePeer)
					remoteName := remotePeer.Name
					// We now know how to get to remoteName: the same
					// way we get to curPeer. Except, if curPeer is
					// the starting peer in which case we know we can
					// reach remoteName directly.
					if curPeer == peer {
						routes[remoteName] = remoteName
					} else {
						routes[remoteName] = routes[curPeer.Name]
					}
				})
		}
	}
	return false, routes
}

func (peer *Peer) ForEachConnectedPeer(establishedAndSymmetric bool, exclude map[PeerName]PeerName, f func(*Peer)) {
	for remoteName, conn := range peer.connections {
		if establishedAndSymmetric && !conn.Established() {
			continue
		}
		if _, found := exclude[remoteName]; found {
			continue
		}
		remotePeer := conn.Remote()
		if remoteConn, found := remotePeer.connections[peer.Name]; !establishedAndSymmetric || (found && remoteConn.Established()) {
			f(remotePeer)
		}
	}
}
