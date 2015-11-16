package ipam

import (
	"github.com/weaveworks/weave/ipam/paxos"
	"github.com/weaveworks/weave/ipam/ring"
	"github.com/weaveworks/weave/ipam/space"
	"github.com/weaveworks/weave/mesh"
	"github.com/weaveworks/weave/net/address"
)

type Status struct {
	Paxos            *paxos.Status
	Range            string
	RangeNumIPs      int
	DefaultSubnet    string
	Ring             *ring.Ring                   // information on ranges owned by all peers
	Space            space.Space                  // more detail on ranges owned by us
	Owned            map[string][]address.Address // addresses by container-ID
	OurName          mesh.PeerName
	Nicknames        map[mesh.PeerName]string `json:"-"`
	IsKnownPeer      func(mesh.PeerName) bool `json:"-"`
	Entries          []EntryStatus
	PendingClaims    []ClaimStatus
	PendingAllocates []string
}

type EntryStatus struct {
	Token   string
	Peer    string
	Version uint32
}

type ClaimStatus struct {
	Ident   string
	Address address.Address
}

func NewStatus(allocator *Allocator, defaultSubnet address.CIDR) *Status {
	if allocator == nil {
		return nil
	}

	var paxosStatus *paxos.Status
	if allocator.paxosTicker != nil {
		paxosStatus = paxos.NewStatus(allocator.paxos)
	}

	resultChan := make(chan *Status)
	allocator.actionChan <- func() {
		resultChan <- &Status{
			paxosStatus,
			allocator.universe.String(),
			int(allocator.universe.Size()),
			defaultSubnet.String(),
			allocator.ring,
			allocator.space,
			allocator.owned,
			allocator.ourName,
			allocator.nicknames,
			allocator.isKnownPeer,
			newEntryStatusSlice(allocator),
			newClaimStatusSlice(allocator),
			newAllocateIdentSlice(allocator)}
	}

	return <-resultChan
}

func newEntryStatusSlice(allocator *Allocator) []EntryStatus {
	var slice []EntryStatus

	if allocator.ring.Empty() {
		return slice
	}

	for _, entry := range allocator.ring.Entries {
		slice = append(slice, EntryStatus{entry.Token.String(), entry.Peer.String(), entry.Version})
	}

	return slice
}

func newClaimStatusSlice(allocator *Allocator) []ClaimStatus {
	var slice []ClaimStatus
	for _, op := range allocator.pendingClaims {
		claim := op.(*claim)
		slice = append(slice, ClaimStatus{claim.ident, claim.addr})
	}
	return slice
}

func newAllocateIdentSlice(allocator *Allocator) []string {
	var slice []string
	for _, op := range allocator.pendingAllocates {
		allocate := op.(*allocate)
		slice = append(slice, allocate.ident)
	}
	return slice
}
