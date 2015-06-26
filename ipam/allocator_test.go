package ipam

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/ipam/address"
	"github.com/weaveworks/weave/router"
)

const (
	testStart1 = "10.0.1.0"
	testStart2 = "10.0.2.0"
	testStart3 = "10.0.3.0"
)

func TestAllocFree(t *testing.T) {
	const (
		container1 = "abcdef"
		container2 = "baddf00d"
		container3 = "b01df00d"
		universe   = "10.0.3.0/26"
		subnet1    = "10.0.3.0/28"
		subnet2    = "10.0.3.32/28"
		testAddr1  = "10.0.3.1"
		testAddr2  = "10.0.3.33"
		spaceSize  = 62 // 64 IP addresses in /26, minus .0 and .63
	)

	alloc, subnet := makeAllocatorWithMockGossip(t, "01:00:00:01:00:00", universe, 1)
	defer alloc.Stop()
	_, cidr1, _ := address.ParseCIDR(subnet1)
	_, cidr2, _ := address.ParseCIDR(subnet2)

	alloc.claimRingForTesting()
	addr1, err := alloc.Allocate(container1, cidr1.HostRange(), nil)
	require.NoError(t, err)
	require.Equal(t, testAddr1, addr1.String(), "address")

	addr2, err := alloc.Allocate(container1, cidr2.HostRange(), nil)
	require.NoError(t, err)
	require.Equal(t, testAddr2, addr2.String(), "address")

	// Ask for another address for a different container and check it's different
	addr1b, _ := alloc.Allocate(container2, cidr1.HostRange(), nil)
	if addr1b.String() == testAddr1 {
		t.Fatalf("Expected different address but got %s", addr1b.String())
	}

	// Ask for the first container again and we should get the same addresses again
	addr1a, _ := alloc.Allocate(container1, cidr1.HostRange(), nil)
	require.Equal(t, testAddr1, addr1a.String(), "address")
	addr2a, _ := alloc.Allocate(container1, cidr2.HostRange(), nil)
	require.Equal(t, testAddr2, addr2a.String(), "address")

	// Now delete the first container, and we should get its addresses back
	require.NoError(t, alloc.Delete(container1))
	addr3, _ := alloc.Allocate(container3, cidr1.HostRange(), nil)
	require.Equal(t, testAddr1, addr3.String(), "address")
	addr4, _ := alloc.Allocate(container3, cidr2.HostRange(), nil)
	require.Equal(t, testAddr2, addr4.String(), "address")

	alloc.ContainerDied(container2)
	alloc.ContainerDied(container3)
	require.Equal(t, address.Offset(spaceSize), alloc.NumFreeAddresses(subnet))
}

func TestBootstrap(t *testing.T) {
	common.InitDefaultLogging(false)
	const (
		donateSize     = 5
		donateStart    = "10.0.1.7"
		ourNameString  = "01:00:00:01:00:00"
		peerNameString = "02:00:00:02:00:00"
	)

	alloc1, subnet := makeAllocatorWithMockGossip(t, ourNameString, testStart1+"/22", 2)
	defer alloc1.Stop()

	// Simulate another peer on the gossip network
	alloc2, _ := makeAllocatorWithMockGossip(t, peerNameString, testStart1+"/22", 2)
	defer alloc2.Stop()

	alloc1.OnGossipBroadcast(alloc2.Encode())

	alloc1.actionChan <- func() { alloc1.tryPendingOps() }

	ExpectBroadcastMessage(alloc1, nil) // alloc1 will try to form consensus
	done := make(chan bool)
	go func() {
		alloc1.Allocate("somecontainer", subnet, nil)
		done <- true
	}()
	time.Sleep(100 * time.Millisecond)
	AssertNothingSent(t, done)

	CheckAllExpectedMessagesSent(alloc1, alloc2)

	alloc1.actionChan <- func() { alloc1.tryPendingOps() }
	AssertNothingSent(t, done)

	CheckAllExpectedMessagesSent(alloc1, alloc2)

	// alloc2 receives paxos update and broadcasts its reply
	ExpectBroadcastMessage(alloc2, nil)
	alloc2.OnGossipBroadcast(alloc1.Encode())

	ExpectBroadcastMessage(alloc1, nil)
	alloc1.OnGossipBroadcast(alloc2.Encode())

	// both nodes will get consensus now so initialize the ring
	ExpectBroadcastMessage(alloc2, nil)
	ExpectBroadcastMessage(alloc2, nil)
	alloc2.OnGossipBroadcast(alloc1.Encode())

	CheckAllExpectedMessagesSent(alloc1, alloc2)

	alloc1.OnGossipBroadcast(alloc2.Encode())
	// now alloc1 should have space

	AssertSent(t, done)

	CheckAllExpectedMessagesSent(alloc1, alloc2)
}

func TestAllocatorClaim(t *testing.T) {
	const (
		container1 = "abcdef"
		container2 = "baddf00d"
		container3 = "b01df00d"
		universe   = "10.0.3.0/30"
		testAddr1  = "10.0.3.1" // first address allocated should be .1 because .0 is network addr
	)

	alloc, subnet := makeAllocatorWithMockGossip(t, "01:00:00:01:00:00", universe, 1)
	defer alloc.Stop()

	alloc.claimRingForTesting()
	addr1, _ := address.ParseIP(testAddr1)

	err := alloc.Claim(container3, addr1, nil)
	require.NoError(t, err)
	// Check we get this address back if we try an allocate
	addr3, _ := alloc.Allocate(container3, subnet, nil)
	require.Equal(t, testAddr1, addr3.String(), "address")
}

func (alloc *Allocator) pause() func() {
	paused := make(chan struct{})
	alloc.actionChan <- func() {
		paused <- struct{}{}
		<-paused
	}
	<-paused
	return func() {
		close(paused)
	}
}

func TestCancel(t *testing.T) {
	common.InitDefaultLogging(false)
	const (
		CIDR = "10.0.1.7/26"
	)

	router := TestGossipRouter{make(map[router.PeerName]chan interface{}), 0.0}

	alloc1, subnet := makeAllocator("01:00:00:02:00:00", CIDR, 2)
	alloc1.SetInterfaces(router.connect(alloc1.ourName, alloc1))

	alloc2, _ := makeAllocator("02:00:00:02:00:00", CIDR, 2)
	alloc2.SetInterfaces(router.connect(alloc2.ourName, alloc2))
	alloc1.claimRingForTesting(alloc1, alloc2)
	alloc2.claimRingForTesting(alloc1, alloc2)

	alloc1.Start()
	alloc2.Start()

	// tell peers about each other
	alloc1.OnGossipBroadcast(alloc2.Encode())

	// Get some IPs, so each allocator has some space
	res1, _ := alloc1.Allocate("foo", subnet, nil)
	common.Debug.Printf("res1 = %s", res1.String())
	res2, _ := alloc2.Allocate("bar", subnet, nil)
	common.Debug.Printf("res2 = %s", res2.String())
	if res1 == res2 {
		require.FailNow(t, "Error: got same ips!")
	}

	// Now we're going to pause alloc2 and ask alloc1
	// for an allocation
	unpause := alloc2.pause()

	// Use up all the IPs that alloc1 owns, so the allocation after this will prompt a request to alloc2
	for i := 0; alloc1.NumFreeAddresses(subnet) > 0; i++ {
		alloc1.Allocate(fmt.Sprintf("tmp%d", i), subnet, nil)
	}
	cancelChan := make(chan bool, 1)
	doneChan := make(chan bool)
	go func() {
		_, ok := alloc1.Allocate("baz", subnet, cancelChan)
		doneChan <- ok == nil
	}()

	AssertNothingSent(t, doneChan)
	time.Sleep(100 * time.Millisecond)
	AssertNothingSent(t, doneChan)

	cancelChan <- true
	unpause()
	if <-doneChan {
		require.FailNow(t, "Error: got result from Allocate")
	}
}

func TestGossipShutdown(t *testing.T) {
	const (
		container1 = "abcdef"
		container2 = "baddf00d"
		universe   = "10.0.3.0/30"
	)

	alloc, subnet := makeAllocatorWithMockGossip(t, "01:00:00:01:00:00", universe, 1)
	defer alloc.Stop()

	alloc.claimRingForTesting()
	alloc.Allocate(container1, subnet, nil)

	alloc.Shutdown()

	_, err := alloc.Allocate(container2, subnet, nil) // trying to allocate after shutdown should fail
	require.False(t, err == nil, "no address")

	CheckAllExpectedMessagesSent(alloc)
}

func TestTransfer(t *testing.T) {
	const (
		cidr = "10.0.1.7/22"
	)
	allocs, router, subnet := makeNetworkOfAllocators(3, cidr)
	alloc1 := allocs[0]
	alloc2 := allocs[1]
	alloc3 := allocs[2] // This will be 'master' and get the first range

	_, err := alloc2.Allocate("foo", subnet, nil)
	require.True(t, err == nil, "Failed to get address")

	_, err = alloc3.Allocate("bar", subnet, nil)
	require.True(t, err == nil, "Failed to get address")

	router.GossipBroadcast(alloc2.Gossip())
	router.flush()
	router.GossipBroadcast(alloc3.Gossip())
	router.flush()
	router.removePeer(alloc2.ourName)
	router.removePeer(alloc3.ourName)
	alloc2.Stop()
	alloc3.Stop()
	router.flush()
	require.NoError(t, alloc1.AdminTakeoverRanges(alloc2.ourName.String()))
	require.NoError(t, alloc1.AdminTakeoverRanges(alloc3.ourName.String()))
	router.flush()

	require.Equal(t, address.Offset(1022), alloc1.NumFreeAddresses(subnet))

	_, err = alloc1.Allocate("foo", subnet, nil)
	require.True(t, err == nil, "Failed to get address")
	alloc1.Stop()
}

func TestFakeRouterSimple(t *testing.T) {
	common.InitDefaultLogging(false)
	const (
		cidr = "10.0.1.7/22"
	)
	allocs, _, subnet := makeNetworkOfAllocators(2, cidr)
	defer stopNetworkOfAllocators(allocs)

	alloc1 := allocs[0]
	//alloc2 := allocs[1]

	_, err := alloc1.Allocate("foo", subnet, nil)
	require.NoError(t, err, "Failed to get address")
}

func TestAllocatorFuzz(t *testing.T) {
	common.InitDefaultLogging(false)
	const (
		firstpass    = 1000
		secondpass   = 20000
		nodes        = 10
		maxAddresses = 1000
		concurrency  = 30
		cidr         = "10.0.1.7/22"
	)
	allocs, _, subnet := makeNetworkOfAllocators(nodes, cidr)
	defer stopNetworkOfAllocators(allocs)

	// Test state
	// For each IP issued we store the allocator
	// that issued it and the name of the container
	// it was issued to.
	type result struct {
		name  string
		alloc int32
		block bool
	}
	stateLock := sync.Mutex{}
	state := make(map[string]result)
	// Keep a list of addresses issued, so we
	// Can pick random ones
	var addrs []string
	numPending := 0

	rand.Seed(0)

	// Remove item from list by swapping it with last
	// and reducing slice length by 1
	rm := func(xs []string, i int32) []string {
		ls := len(xs) - 1
		xs[i] = xs[ls]
		return xs[:ls]
	}

	// Do a Allocate and check the address
	// is unique.  Needs a unique container
	// name.
	allocate := func(name string) {
		stateLock.Lock()
		if len(addrs)+numPending >= maxAddresses {
			stateLock.Unlock()
			return
		}
		numPending++
		stateLock.Unlock()

		allocIndex := rand.Int31n(nodes)
		alloc := allocs[allocIndex]
		//common.Info.Printf("Allocate: asking allocator %d", allocIndex)
		addr, err := alloc.Allocate(name, subnet, nil)

		if err != nil {
			panic(fmt.Sprintf("Could not allocate addr"))
		}

		//common.Info.Printf("Allocate: got address %s for name %s", addr, name)
		addrStr := addr.String()

		stateLock.Lock()
		defer stateLock.Unlock()

		if res, existing := state[addrStr]; existing {
			panic(fmt.Sprintf("Dup found for address %s - %s and %s", addrStr,
				name, res.name))
		}

		state[addrStr] = result{name, allocIndex, false}
		addrs = append(addrs, addrStr)
		numPending--
	}

	// Free a random address.
	free := func() {
		stateLock.Lock()
		if len(addrs) == 0 {
			stateLock.Unlock()
			return
		}
		// Delete an existing allocation
		// Pick random addr
		addrIndex := rand.Int31n(int32(len(addrs)))
		addr := addrs[addrIndex]
		res := state[addr]
		if res.block {
			stateLock.Unlock()
			return
		}
		addrs = rm(addrs, addrIndex)
		delete(state, addr)
		stateLock.Unlock()

		alloc := allocs[res.alloc]
		//common.Info.Printf("Freeing %s (%s) on allocator %d", res.name, addr, res.alloc)

		oldAddr, err := address.ParseIP(addr)
		if err != nil {
			panic(err)
		}
		require.NoError(t, alloc.Free(res.name, oldAddr))
	}

	// Do a Allocate on an existing container & allocator
	// and check we get the right answer.
	allocateAgain := func() {
		stateLock.Lock()
		addrIndex := rand.Int31n(int32(len(addrs)))
		addr := addrs[addrIndex]
		res := state[addr]
		if res.block {
			stateLock.Unlock()
			return
		}
		res.block = true
		state[addr] = res
		stateLock.Unlock()
		alloc := allocs[res.alloc]

		//common.Info.Printf("Asking for %s (%s) on allocator %d again", res.name, addr, res.alloc)

		newAddr, _ := alloc.Allocate(res.name, subnet, nil)
		oldAddr, _ := address.ParseIP(addr)
		if newAddr != oldAddr {
			panic(fmt.Sprintf("Got different address for repeat request for %s: %s != %s", res.name, newAddr, oldAddr))
		}

		stateLock.Lock()
		res.block = false
		state[addr] = res
		stateLock.Unlock()
	}

	// Run function _f_ _iterations_ times, in _concurrency_
	// number of goroutines
	doConcurrentIterations := func(iterations int, f func(int)) {
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

	// First pass, just allocate a bunch of ips
	doConcurrentIterations(firstpass, func(iteration int) {
		name := fmt.Sprintf("first%d", iteration)
		allocate(name)
	})

	// Second pass, random ask for more allocations,
	// or remove existing ones, or ask for allocation
	// again.
	doConcurrentIterations(secondpass, func(iteration int) {
		r := rand.Float32()
		switch {
		case 0.0 <= r && r < 0.4:
			// Ask for a new allocation
			name := fmt.Sprintf("second%d", iteration)
			allocate(name)

		case (0.4 <= r && r < 0.8):
			// free a random addr
			free()

		case 0.8 <= r && r < 1.0:
			// ask for an existing name again, check we get same ip
			allocateAgain()
		}
	})
}

func TestGossipSkew(t *testing.T) {
	alloc1, _ := makeAllocatorWithMockGossip(t, "01:00:00:01:00:00", "10.0.1.0/22", 2)
	defer alloc1.Stop()
	alloc2, _ := makeAllocatorWithMockGossip(t, "02:00:00:02:00:00", "10.0.1.0/22", 2)
	alloc2.now = func() time.Time { return time.Now().Add(time.Hour * 2) }
	defer alloc2.Stop()

	if _, err := alloc1.OnGossipBroadcast(alloc2.Encode()); err == nil {
		t.Fail()
	}
}
