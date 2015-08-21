package nameserver

import (
	"fmt"
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/router"
	"github.com/weaveworks/weave/testing/gossip"
)

func makeQuarantineNetwork(size int) ([]*QuarantineManager, *gossip.TestRouter) {
	gossipRouter := gossip.NewTestRouter(0.0)
	managers := make([]*QuarantineManager, size)

	for i := 0; i < size; i++ {
		name, _ := router.PeerNameFromString(fmt.Sprintf("%02d:00:00:02:00:00", i))
		manager := &QuarantineManager{}
		manager.SetGossip(gossipRouter.Connect(name, manager))
		managers[i] = manager
	}

	return managers, gossipRouter
}

type quarantine struct {
	id          string
	peername    router.PeerName
	containerid string
	deleted     bool
}

func TestQuarantines(t *testing.T) {
	common.SetLogLevel("debug")

	lookupTimeout := 20
	managers, grouter := makeQuarantineNetwork(50)
	quarantines := []*quarantine{}

	check := func(manager *QuarantineManager, q *quarantine) {
		for i := 0; i < lookupTimeout; i++ {
			if q.deleted && !manager.filter(&Entry{ContainerID: q.containerid}) &&
				!manager.filter(&Entry{Origin: q.peername}) {
				break
			}
			if !q.deleted && manager.filter(&Entry{ContainerID: q.containerid}) &&
				manager.filter(&Entry{Origin: q.peername}) {
				break
			}
			time.Sleep(1 * time.Millisecond)
		}

		require.Equal(t, !q.deleted, manager.filter(&Entry{ContainerID: q.containerid}))
		//require.Equal(t, !q.deleted, manager.filter(&Entry{Origin: q.peername}))
	}

	add := func() {
		manager := managers[rand.Intn(len(managers))]
		containerid := strconv.FormatInt(rand.Int63(), 16)
		peername, _ := router.PeerNameFromString(fmt.Sprintf("%02d:00:00:02:00:00", rand.Intn(len(managers))))
		id, err := manager.Add(containerid, peername, time.Hour)
		require.Nil(t, err)
		q := &quarantine{id, peername, containerid, false}
		quarantines = append(quarantines, q)
		check(manager, q)
	}

	delete := func() {
		if len(quarantines) <= 0 {
			return
		}
		q := quarantines[rand.Intn(len(quarantines))]
		if q.deleted {
			return
		}
		manager := managers[rand.Intn(len(managers))]

		// due to this being eventually-consistent, delete might not work immediately
		var err error
		for i := 0; i < lookupTimeout; i++ {
			err = manager.Delete(q.id)
			if err == nil {
				break
			}
			time.Sleep(1 * time.Millisecond)
		}
		require.Nil(t, err)

		q.deleted = true
		check(manager, q)
	}

	filter := func() {
		if len(quarantines) <= 0 {
			return
		}
		q := quarantines[rand.Intn(len(quarantines))]
		manager := managers[rand.Intn(len(managers))]
		check(manager, q)
	}

	for i := 0; i < 1000; i++ {
		if i%10 == 0 {
			grouter.Flush()
		}

		r := rand.Float32()
		switch {
		case r < 0.2:
			add()

		case 0.2 <= r && r < 0.4:
			delete()

		case 0.4 <= r:
			filter()
		}
	}
}
