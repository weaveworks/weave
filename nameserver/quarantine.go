package nameserver

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"sync"
	"time"

	. "github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/router"
)

type Quarantine struct {
	ID          string
	ValidUntil  int64
	ContainerID string
	Peer        router.PeerName
	Version     int64
}

type Quarantines []Quarantine

type QuarantineManager struct {
	sync.RWMutex
	quarantines Quarantines
	gossip      router.Gossip
}

func (q1 *Quarantine) merge(q2 Quarantine) {
	if q2.Version > q1.Version {
		q1.ValidUntil = q2.ValidUntil
	}
}

func (qs *Quarantines) merge(incomings *Quarantines) Quarantines {
	var (
		newQuarantines Quarantines
		i              = 0
	)

	for _, incoming := range *incomings {
		for i < len(*qs) && (*qs)[i].ID < incoming.ID {
			i++
		}
		if i < len(*qs) && (*qs)[i].ID == incoming.ID {
			(*qs)[i].merge(incoming)
			continue
		}
		*qs = append(*qs, Quarantine{})
		copy((*qs)[i+1:], (*qs)[i:])
		(*qs)[i] = incoming
		newQuarantines = append(newQuarantines, incoming)
	}

	return newQuarantines
}

func (qs *Quarantines) add(containerID string, peername router.PeerName, duration time.Duration) Quarantine {

	q := Quarantine{
		ID:          strconv.FormatInt(rand.Int63(), 16),
		ValidUntil:  now() + int64(duration/time.Second),
		ContainerID: containerID,
		Peer:        peername,
		Version:     0,
	}

	i := sort.Search(len(*qs), func(i int) bool {
		return (*qs)[i].ID > q.ID
	})

	*qs = append(*qs, Quarantine{})
	copy((*qs)[i+1:], (*qs)[i:])
	(*qs)[i] = q

	return (*qs)[i]
}

func (qs *Quarantines) Merge(other router.GossipData) {
	qs.merge(other.(*Quarantines))
}

func (qs *Quarantines) Encode() [][]byte {
	buf := &bytes.Buffer{}
	if err := gob.NewEncoder(buf).Encode(qs); err != nil {
		panic(err)
	}
	return [][]byte{buf.Bytes()}
}

func (qm *QuarantineManager) SetGossip(gossip router.Gossip) {
	qm.gossip = gossip
}

func (qm *QuarantineManager) List() Quarantines {
	qm.RLock()
	defer qm.RUnlock()

	result := Quarantines{}
	for _, q := range qm.quarantines {
		if q.ValidUntil > now() {
			result = append(result, q)
		}
	}
	return result
}

func (qm *QuarantineManager) Add(containerID string, peername router.PeerName, duration time.Duration) (string, error) {
	Log.Infof("[quarantine] Adding quarantine container=%s, peername=%s, duration=%s",
		containerID, peername.String(), duration.String())

	qm.Lock()
	q := qm.quarantines.add(containerID, peername, duration)
	qm.Unlock()

	if qm.gossip != nil {
		return q.ID, qm.gossip.GossipBroadcast(&Quarantines{q})
	}
	return q.ID, nil
}

func (qm *QuarantineManager) Delete(ident string) error {
	qm.Lock()
	defer qm.Unlock()

	Log.Infof("[quarantine] Deleting quarantine %s", ident)

	i := sort.Search(len(qm.quarantines), func(i int) bool {
		return qm.quarantines[i].ID >= ident
	})
	if i == len(qm.quarantines) || qm.quarantines[i].ID != ident {
		return fmt.Errorf("Not found")
	}
	qm.quarantines[i].Version++
	qm.quarantines[i].ValidUntil = now()

	if qm.gossip != nil {
		return qm.gossip.GossipBroadcast(&Quarantines{qm.quarantines[i]})
	}
	return nil
}

func (qm *QuarantineManager) filter(entry *Entry) bool {
	n := now()
	for _, q := range qm.quarantines {
		if q.ValidUntil <= n {
			continue
		}
		if q.ContainerID == entry.ContainerID {
			return true
		}
		if q.Peer == entry.Origin {
			return true
		}
	}
	return false
}

func (qm *QuarantineManager) Gossip() router.GossipData {
	qm.RLock()
	defer qm.RUnlock()
	result := make(Quarantines, len(qm.quarantines))
	copy(result, qm.quarantines)
	return &result
}

func (qm *QuarantineManager) OnGossipUnicast(sender router.PeerName, msg []byte) error {
	return nil
}

func (qm *QuarantineManager) receiveGossip(msg []byte) (router.GossipData, router.GossipData, error) {
	var qs Quarantines
	if err := gob.NewDecoder(bytes.NewReader(msg)).Decode(&qs); err != nil {
		return nil, nil, err
	}

	qm.Lock()
	defer qm.Unlock()

	newEntries := qm.quarantines.merge(&qs)
	return &newEntries, &qs, nil
}

// merge received data into state and return "everything new I've
// just learnt", or nil if nothing in the received data was new
func (qm *QuarantineManager) OnGossip(msg []byte) (router.GossipData, error) {
	newEntries, _, err := qm.receiveGossip(msg)
	return newEntries, err
}

// merge received data into state and return a representation of
// the received data, for further propagation
func (qm *QuarantineManager) OnGossipBroadcast(_ router.PeerName, msg []byte) (router.GossipData, error) {
	_, entries, err := qm.receiveGossip(msg)
	return entries, err
}
