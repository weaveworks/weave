package main

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	peerName1 = "01:00:00:00:00:00"
	peerName2 = "02:00:00:00:00:00"
	peerName3 = "03:00:00:00:00:00"
	nodeName1 = "fake-node-1"
	nodeName2 = "fake-node-2"

	testIters = 1000
)

func TestPeerListBasic(t *testing.T) {
	c := NewSimpleClientset()

	cml := newConfigMapAnnotations(configMapNamespace, configMapName, c)
	err := cml.Init()
	require.NoError(t, err)

	list1, err := addMyselfToPeerList(cml, c, peerName1, nodeName1)
	require.NoError(t, err)
	require.Equal(t, 1, len(list1.Peers))
	list2, err := addMyselfToPeerList(cml, c, peerName2, nodeName2)
	require.NoError(t, err)
	require.Equal(t, 2, len(list2.Peers))

	check1, err := checkIamInPeerList(cml, c, peerName1)
	require.NoError(t, err)
	require.Equal(t, true, check1)
	check3, err := checkIamInPeerList(cml, c, peerName3)
	require.NoError(t, err)
	require.Equal(t, false, check3)

	storedPeerList, err := cml.GetPeerList()
	require.NoError(t, err)

	storedPeerList.remove(peerName1)
	require.Equal(t, 1, len(storedPeerList.Peers))
	err = cml.UpdatePeerList(*storedPeerList)
	require.NoError(t, err)
	check1, err = checkIamInPeerList(cml, c, peerName1)
	require.NoError(t, err)
	require.Equal(t, false, check1)
}

// Run function _f_ _iterations_ times, in _concurrency_
// number of goroutines
func doConcurrentIterations(iterations, concurrency int, f func(int)) {
	iterationsPerThread := iterations / concurrency

	wg := sync.WaitGroup{}
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(j int) {
			defer wg.Done()
			for k := 0; k < iterationsPerThread; k++ {
				f((j * iterationsPerThread) + k)
			}
		}(i)
	}
	wg.Wait()
}

func TestPeerListFuzz(t *testing.T) {
	const (
		testIters   = 1000
		maxNodes    = 100
		concurrency = 5
	)
	peerName := func(i int) string { return fmt.Sprintf("%02d:00:00:02:00:00", i) }
	nodeName := func(i int) string { return fmt.Sprintf("node%d", i) }

	stateLock := sync.Mutex{}
	nodes := []int{}
	nodeMap := map[int]bool{} // true if an operation is pending

	c := NewSimpleClientset()

	addNode := func() {
		i := rand.Intn(maxNodes)
		// add to test state first, then to Kubernetes
		stateLock.Lock()
		pending, found := nodeMap[i]
		if !found {
			nodes = append(nodes, i)
		}
		nodeMap[i] = true
		stateLock.Unlock()
		if pending { // someone else is adding or deleting this node
			return
		}

		cml := newConfigMapAnnotations(configMapNamespace, configMapName, c)
		err := cml.Init()
		require.NoError(t, err)
		_, err = addMyselfToPeerList(cml, c, peerName(i), nodeName(i))
		require.NoError(t, err)

		stateLock.Lock()
		nodeMap[i] = false
		stateLock.Unlock()
	}

	removeNode := func() {
		stateLock.Lock()
		if len(nodes) == 0 { // can't remove if there aren't any nodes
			stateLock.Unlock()
			return
		}
		pos := rand.Intn(len(nodes))
		i := nodes[pos]
		if pending, found := nodeMap[i]; found {
			if pending {
				stateLock.Unlock()
				return // someone else is adding or deleting this node
			}
			nodeMap[i] = true
		} else {
			t.Errorf("missing entry from node map: %d", i)
		}
		stateLock.Unlock()

		// remove from Kubernetes first, then from test state
		cml := newConfigMapAnnotations(configMapNamespace, configMapName, c)
		require.NoError(t, cml.Init())
		storedPeerList, err := cml.GetPeerList()
		require.NoError(t, err)
		found := storedPeerList.contains(peerName(i))
		require.True(t, found, "peer %d not found in stored list", i)

		_, err = reclaimPeer(mockWeave{}, cml, peerName(i), fmt.Sprintf("deleter-%d", i))
		require.NoError(t, err)

		require.NoError(t, cml.Init())
		storedPeerList, err = cml.GetPeerList()
		require.NoError(t, err)
		stateLock.Lock()
		delete(nodeMap, i)
		for pos = 0; pos < len(nodes); pos++ {
			if nodes[pos] == i {
				// Remove item from list by swapping it with last and reducing slice length by 1
				nodes[pos] = nodes[len(nodes)-1]
				nodes = nodes[:len(nodes)-1]
				break
			}
		}
		stateLock.Unlock()
	}

	doConcurrentIterations(testIters, concurrency, func(iteration int) {
		r := rand.Float32()
		switch {
		case 0.0 <= r && r < 0.4:
			addNode()

		case (0.4 <= r && r < 0.8):
			removeNode()
		}
	})

	cml := newConfigMapAnnotations(configMapNamespace, configMapName, c)
	err := cml.Init()
	require.NoError(t, err)
	storedPeerList, err := cml.GetPeerList()
	require.NoError(t, err)
	require.Equal(t, len(storedPeerList.Peers), len(nodes))
}

type mockWeave struct{}

func (mockWeave) RmPeer(peerName string) (string, error) {
	return "", nil
}
