package ipam

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"time"

	lg "github.com/zettio/weave/common"
	"github.com/zettio/weave/ipam/ring"
	"github.com/zettio/weave/ipam/space"
	"github.com/zettio/weave/ipam/utils"
	"github.com/zettio/weave/router"
)

const (
	tombstoneTimeout = 14 * 24 * time.Hour
)

// Kinds of message we can unicast to other peers
const (
	msgSpaceRequest = iota
	msgLeaderElected
	msgRingUpdate
)

type pendingAllocation struct {
	resultChan chan<- net.IP
	Ident      string
}

// Allocator brings together Ring and space.Set, and does the nessecary plumbing.
type Allocator struct {
	actionChan    chan<- func()
	ourName       router.PeerName
	universeStart net.IP
	universeSize  uint32
	universeLen   int        // length of network prefix (e.g. 24 for a /24 network)
	ring          *ring.Ring // it's for you!
	spaceSet      space.Set
	owned         map[string][]net.IP // who owns what address, indexed by container-ID
	pending       []pendingAllocation
	claims        claimList
	gossip        router.Gossip
	shuttingDown  bool
}

// NewAllocator creats and initialises a new Allocator
func NewAllocator(ourName router.PeerName, universeCIDR string) (*Allocator, error) {
	_, universeNet, err := net.ParseCIDR(universeCIDR)
	if err != nil {
		return nil, err
	}
	if universeNet.IP.To4() == nil {
		return nil, errors.New("Non-IPv4 address not supported")
	}
	// Get the size of the network from the mask
	ones, bits := universeNet.Mask.Size()
	var universeSize uint32 = 1 << uint(bits-ones)
	if universeSize < 4 {
		return nil, errors.New("Allocation universe too small")
	}
	alloc := &Allocator{
		ourName:       ourName,
		universeStart: universeNet.IP,
		universeSize:  universeSize,
		universeLen:   ones,
		ring:          ring.New(utils.Add(universeNet.IP, 1), utils.Add(universeNet.IP, universeSize-1), ourName),
		owned:         make(map[string][]net.IP),
	}
	return alloc, nil
}

func (alloc *Allocator) string() string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("Allocator universe %s+%d\n", alloc.universeStart, alloc.universeSize))
	buf.WriteString(alloc.ring.String())
	buf.WriteString(alloc.spaceSet.String())
	buf.WriteString("\nPending requests for ")
	for _, pending := range alloc.pending {
		buf.WriteString(pending.Ident)
		buf.WriteString(", ")
	}
	return buf.String()
}

func (alloc *Allocator) checkPendingAllocations() {
	i := 0
	for ; i < len(alloc.pending); i++ {
		if !alloc.tryAllocateFor(alloc.pending[i].Ident, alloc.pending[i].resultChan) {
			break
		}
	}
	alloc.pending = alloc.pending[i:]
}

// Fairly quick check of what's going on; whether requests should now be
// replied to, etc.
func (alloc *Allocator) considerOurPosition() {
	alloc.checkPendingAllocations()
	alloc.checkClaims()
}

func (alloc *Allocator) electLeaderIfNecessary() {
	if !alloc.ring.Empty() {
		return
	}
	leader := alloc.gossip.(router.Leadership).LeaderElect()
	alloc.debugln("Elected leader:", leader)
	if leader == alloc.ourName {
		// I'm the winner; take control of the whole universe
		alloc.ring.ClaimItAll()
		alloc.considerNewSpaces()
		alloc.infof("I was elected leader of the universe\n%s", alloc.string())
		alloc.gossip.GossipBroadcast(alloc.Gossip())
		alloc.considerOurPosition()
	} else {
		alloc.sendRequest(leader, msgLeaderElected)
	}
}

// return true if the request is completed, false if pending
func (alloc *Allocator) tryAllocateFor(ident string, resultChan chan<- net.IP) bool {
	// If we have previously stored an address for this container, return it.
	if addrs, found := alloc.owned[ident]; found && len(addrs) > 0 {
		resultChan <- addrs[0]
		return true
	}
	if addr := alloc.spaceSet.Allocate(); addr != nil {
		alloc.debugln("Allocated", addr, "for", ident)
		alloc.addOwned(ident, addr)
		resultChan <- addr
		return true
	}

	// out of space
	if donor, err := alloc.ring.ChoosePeerToAskForSpace(); err == nil {
		alloc.debugln("Decided to ask peer", donor, "for space")
		alloc.sendRequest(donor, msgSpaceRequest)
	}

	return false
}

func (alloc *Allocator) handleLeaderElected() error {
	// some other peer decided we were the leader:
	// if we already have tokens then they didn't get the memo; repeat
	if !alloc.ring.Empty() {
		alloc.gossip.GossipBroadcast(alloc.Gossip())
	} else {
		// re-run the election here to avoid races
		alloc.electLeaderIfNecessary()
	}
	return nil
}

func (alloc *Allocator) sendRequest(dest router.PeerName, kind byte) {
	msg := router.Concat([]byte{kind}, alloc.ring.GossipState())
	alloc.gossip.GossipUnicast(dest, msg)
}

func (alloc *Allocator) updateRing(msg []byte) error {
	err := alloc.ring.UpdateRing(msg)
	alloc.considerNewSpaces()
	alloc.considerOurPosition()
	return err
}

func (alloc *Allocator) donateSpace(to router.PeerName) {
	// No matter what we do, we'll send a unicast gossip
	// of our ring back to tha chap who asked for space.
	// This serves to both tell him of any space we might
	// have given him, or tell him where he might find some
	// more.
	defer alloc.sendRequest(to, msgRingUpdate)

	alloc.debugln("Peer", to, "asked me for space")
	start, size, ok := alloc.spaceSet.GiveUpSpace()
	if !ok {
		free := alloc.spaceSet.NumFreeAddresses()
		utils.Assert(free == 0,
			fmt.Sprintf("Couldn't give up space but I have %d free addresses", free))
		alloc.debugln("No space to give to peer", to)
		return
	}
	end := utils.IntIP4(utils.IP4int(start) + size)
	alloc.debugln("Giving range", start, end, size, "to", to)
	alloc.ring.GrantRangeToHost(start, end, to)
}

// considerNewSpaces iterates through ranges in the ring
// and ensures we have spaces for them.  It only ever adds
// new spaces, as the invariants in the ring ensure we never
// have spaces taken away from us against our will.
func (alloc *Allocator) considerNewSpaces() {
	ownedRanges := alloc.ring.OwnedRanges()
	for _, r := range ownedRanges {
		size := uint32(utils.Subtract(r.End, r.Start))
		s, exists := alloc.spaceSet.Get(r.Start)
		if !exists {
			alloc.debugf("Found new space [%s, %s)", r.Start, r.End)
			alloc.spaceSet.AddSpace(space.Space{Start: r.Start, Size: size})
			continue
		}

		if s.Size < size {
			alloc.debugf("Growing space starting at %s to %d", s.Start, size)
			s.Grow(size)
		}
	}
}

func (alloc *Allocator) assertInvariants() {
	// We need to ensure all ranges the ring thinks we own have
	// a corresponding space in the space set, and vice versa
	ranges := alloc.ring.OwnedRanges()
	spaces := alloc.spaceSet.Spaces()

	utils.Assert(len(ranges) == len(spaces), "Ring and SpaceSet are out of sync!")

	for i := 0; i < len(ranges); i++ {
		r := ranges[i]
		s := spaces[i]

		rSize := uint32(utils.Subtract(r.End, r.Start))
		utils.Assert(s.Start.Equal(r.Start) && s.Size == rSize,
			fmt.Sprintf("Range starting at %s out of sync with space set!", r.Start))
	}
}

func (alloc *Allocator) reportFreeSpace() {
	spaces := alloc.spaceSet.Spaces()

	for _, s := range spaces {
		alloc.ring.ReportFree(s.Start, s.NumFreeAddresses())
	}
}

func (alloc *Allocator) errorln(args ...interface{}) {
	lg.Error.Println(append([]interface{}{fmt.Sprintf("[allocator %s]:", alloc.ourName)}, args...)...)
}
func (alloc *Allocator) infof(fmt string, args ...interface{}) {
	lg.Info.Printf("[allocator %s] "+fmt, append([]interface{}{alloc.ourName}, args...)...)
}
func (alloc *Allocator) debugln(args ...interface{}) {
	lg.Debug.Println(append([]interface{}{fmt.Sprintf("[allocator %s]:", alloc.ourName)}, args...)...)
}
func (alloc *Allocator) debugf(fmt string, args ...interface{}) {
	lg.Debug.Printf("[allocator %s] "+fmt, append([]interface{}{alloc.ourName}, args...)...)
}
