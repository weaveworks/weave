package sortinghat

import (
	"github.com/zettio/weave/router"
	"log"
	"net"
)

type Allocator struct {
	ourName router.PeerName
	gossip  router.GossipCommsProvider
	pss     PeerSpaceSet
}

func NewAllocator(ourName router.PeerName, gossip router.GossipCommsProvider, startAddr net.IP, poolSize int) *Allocator {
	peerSpace := NewPeerSpace(ourName)
	if poolSize > 0 {
		peerSpace.AddSpace(NewSpace(startAddr, uint32(poolSize)))
	}

	return &Allocator{
		gossip:  gossip,
		ourName: ourName,
		pss: PeerSpaceSet{
			spacesets: map[router.PeerName]*PeerSpace{ourName: peerSpace},
		},
	}
}

func (alloc *Allocator) ConsiderOurPosition() {
	// Rule: if we have no IP space, pick the peer with the most available space and request some
	if alloc.pss.spacesets[alloc.ourName].NumFreeAddresses() == 0 {
		var best *PeerSpace = nil
		var bestNumFree uint32 = 0
		for _, spaceset := range alloc.pss.spacesets {
			if num := spaceset.NumFreeAddresses(); num > bestNumFree {
				bestNumFree = num
				best = spaceset
			}
		}
		if best != nil {
			log.Println("Decided to ask peer", best.PeerName, "for space")
		}
	}
}

func (alloc *Allocator) AllocateFor(ident string) net.IP {
	return nil
}

func (alloc *Allocator) Free(addr net.IP) {
}

func (alloc *Allocator) String() string {
	return "something"
}

// GossipDelegate methods
func (alloc *Allocator) NotifyMsg(msg []byte) {
	log.Printf("NotifyMsg: %+v\n", msg)
}

func (alloc *Allocator) GetBroadcasts(overhead, limit int) [][]byte {
	log.Printf("GetBroadcasts: %d %d\n", overhead, limit)
	return nil
}

func (alloc *Allocator) LocalState(join bool) []byte {
	log.Printf("LocalState: %t\n", join)
	if buf, err := alloc.pss.Encode(); err == nil {
		return buf
	} else {
		log.Println("Error", err)
	}
	return nil
}

func (alloc *Allocator) MergeRemoteState(buf []byte, join bool) {
	log.Printf("MergeRemoteState: %t %d bytes\n", join, len(buf))
	alloc.pss.DecodeUpdate(buf)
	alloc.ConsiderOurPosition()
}
