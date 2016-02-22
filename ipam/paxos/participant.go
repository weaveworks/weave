package paxos

import "github.com/weaveworks/mesh"

type Participant interface {
	GossipState() GossipState
	Update(from GossipState) bool
	Propose()
	Think() bool
	Consensus() (bool, AcceptedValue)
}

func NewParticipant(name mesh.PeerName, uid mesh.PeerUID, quorum uint) Participant {
	switch quorum {
	case 0:
		return &Observer{}
	default:
		return NewNode(name, uid, quorum)
	}
}

type Observer struct {
}

func (observer *Observer) GossipState() GossipState {
	return nil
}

func (observer *Observer) Update(from GossipState) bool {
	return false
}

func (observer *Observer) Propose() {
}

func (observer *Observer) Think() bool {
	return false
}

func (observer *Observer) Consensus() (bool, AcceptedValue) {
	return false, AcceptedValue{}
}
