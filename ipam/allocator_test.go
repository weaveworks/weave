package ipam

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/net/address"
	"github.com/weaveworks/weave/testing/gossip"
)

const (
	testStart1 = "10.0.1.0"
	testStart2 = "10.0.2.0"
	testStart3 = "10.0.3.0"
)

func returnFalse() bool { return false }

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
	addr1, err := alloc.Allocate(container1, cidr1.HostRange(), returnFalse)
	require.NoError(t, err)
	require.Equal(t, testAddr1, addr1.String(), "address")

	addr2, err := alloc.Allocate(container1, cidr2.HostRange(), returnFalse)
	require.NoError(t, err)
	require.Equal(t, testAddr2, addr2.String(), "address")

	// Ask for another address for a different container and check it's different
	addr1b, _ := alloc.Allocate(container2, cidr1.HostRange(), returnFalse)
	if addr1b.String() == testAddr1 {
		t.Fatalf("Expected different address but got %s", addr1b.String())
	}

	// Ask for the first container again and we should get the same addresses again
	addr1a, _ := alloc.Allocate(container1, cidr1.HostRange(), returnFalse)
	require.Equal(t, testAddr1, addr1a.String(), "address")
	addr2a, _ := alloc.Allocate(container1, cidr2.HostRange(), returnFalse)
	require.Equal(t, testAddr2, addr2a.String(), "address")

	// Now delete the first container, and we should get its addresses back
	require.NoError(t, alloc.Delete(container1))
	addr3, _ := alloc.Allocate(container3, cidr1.HostRange(), returnFalse)
	require.Equal(t, testAddr1, addr3.String(), "address")
	addr4, _ := alloc.Allocate(container3, cidr2.HostRange(), returnFalse)
	require.Equal(t, testAddr2, addr4.String(), "address")

	alloc.ContainerDied(container2)

	// Resurrect
	addr1c, err := alloc.Allocate(container2, cidr1.HostRange(), returnFalse)
	require.NoError(t, err)
	require.Equal(t, addr1b, addr1c, "address")

	alloc.ContainerDied(container3)
	alloc.Encode() // sync up
	// Move the clock forward and clear out the dead container
	alloc.actionChan <- func() { alloc.now = func() time.Time { return time.Now().Add(containerDiedTimeout * 2) } }
	alloc.actionChan <- func() { alloc.removeDeadContainers() }
	require.Equal(t, address.Offset(spaceSize-1), alloc.NumFreeAddresses(subnet))
}

func TestBootstrap(t *testing.T) {
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

	alloc1.OnGossipBroadcast(alloc2.ourName, alloc2.Encode())

	alloc1.actionChan <- func() { alloc1.tryPendingOps() }

	ExpectBroadcastMessage(alloc1, nil) // alloc1 will try to form consensus
	done := make(chan bool)
	go func() {
		alloc1.Allocate("somecontainer", subnet, returnFalse)
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
	alloc2.OnGossipBroadcast(alloc1.ourName, alloc1.Encode())

	ExpectBroadcastMessage(alloc1, nil)
	alloc1.OnGossipBroadcast(alloc2.ourName, alloc2.Encode())

	// both nodes will get consensus now so initialize the ring
	ExpectBroadcastMessage(alloc2, nil)
	ExpectBroadcastMessage(alloc2, nil)
	alloc2.OnGossipBroadcast(alloc1.ourName, alloc1.Encode())

	CheckAllExpectedMessagesSent(alloc1, alloc2)

	alloc1.OnGossipBroadcast(alloc2.ourName, alloc2.Encode())
	// now alloc1 should have space

	AssertSent(t, done)

	CheckAllExpectedMessagesSent(alloc1, alloc2)
}

func TestAllocatorClaim(t *testing.T) {
	const (
		container1 = "abcdef"
		container3 = "b01df00d"
		universe   = "10.0.3.0/24"
		testAddr1  = "10.0.3.2"
		testAddr2  = "10.0.4.2"
	)

	allocs, _, subnet := makeNetworkOfAllocators(2, universe)
	alloc := allocs[1]
	defer alloc.Stop()
	addr1, _ := address.ParseIP(testAddr1)

	// First claim should trigger "dunno, I'm going to wait"
	err := alloc.Claim(container3, addr1, true)
	require.NoError(t, err)

	// Do one allocate to ensure paxos is all done
	alloc.Allocate("unused", subnet, returnFalse)
	// Do an allocate on the other peer, which we will try to claim later
	addrx, err := allocs[0].Allocate(container1, subnet, returnFalse)

	// Now try the claim again
	err = alloc.Claim(container3, addr1, true)
	require.NoError(t, err)
	// Check we get this address back if we try an allocate
	addr3, _ := alloc.Allocate(container3, subnet, returnFalse)
	require.Equal(t, testAddr1, addr3.String(), "address")
	// one more claim should still work
	err = alloc.Claim(container3, addr1, true)
	require.NoError(t, err)
	// claim for a different container should fail
	err = alloc.Claim(container1, addr1, true)
	require.Error(t, err)
	// claiming the address allocated on the other peer should fail
	err = alloc.Claim(container1, addrx, true)
	require.Error(t, err, "claiming address allocated on other peer should fail")
	// Check an address outside of our universe
	addr2, _ := address.ParseIP(testAddr2)
	err = alloc.Claim(container1, addr2, true)
	require.NoError(t, err)
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
	const (
		CIDR = "10.0.1.7/26"
	)

	router := gossip.NewTestRouter(0.0)

	alloc1, subnet := makeAllocator("01:00:00:02:00:00", CIDR, 2)
	alloc1.SetInterfaces(router.Connect(alloc1.ourName, alloc1))

	alloc2, _ := makeAllocator("02:00:00:02:00:00", CIDR, 2)
	alloc2.SetInterfaces(router.Connect(alloc2.ourName, alloc2))
	alloc1.claimRingForTesting(alloc1, alloc2)
	alloc2.claimRingForTesting(alloc1, alloc2)

	alloc1.Start()
	alloc2.Start()

	// tell peers about each other
	alloc1.OnGossipBroadcast(alloc2.ourName, alloc2.Encode())

	// Get some IPs, so each allocator has some space
	res1, _ := alloc1.Allocate("foo", subnet, returnFalse)
	common.Log.Debugf("res1 = %s", res1.String())
	res2, _ := alloc2.Allocate("bar", subnet, returnFalse)
	common.Log.Debugf("res2 = %s", res2.String())
	if res1 == res2 {
		require.FailNow(t, "Error: got same ips!")
	}

	// Now we're going to pause alloc2 and ask alloc1
	// for an allocation
	unpause := alloc2.pause()

	// Use up all the IPs that alloc1 owns, so the allocation after this will prompt a request to alloc2
	for i := 0; alloc1.NumFreeAddresses(subnet) > 0; i++ {
		alloc1.Allocate(fmt.Sprintf("tmp%d", i), subnet, returnFalse)
	}
	cancelChan := make(chan bool, 1)
	doneChan := make(chan bool)
	go func() {
		_, ok := alloc1.Allocate("baz", subnet,
			func() bool {
				select {
				case <-cancelChan:
					return true
				default:
					return false
				}
			})
		doneChan <- ok == nil
	}()

	time.Sleep(100 * time.Millisecond)
	AssertNothingSent(t, doneChan)

	cancelChan <- true
	unpause()
	if <-doneChan {
		require.FailNow(t, "Error: got result from Allocate")
	}
}

func TestCancelOnDied(t *testing.T) {
	const (
		CIDR       = "10.0.1.7/26"
		container1 = "abcdef"
	)

	router := gossip.NewTestRouter(0.0)
	alloc1, subnet := makeAllocator("01:00:00:02:00:00", CIDR, 2)
	alloc1.SetInterfaces(router.Connect(alloc1.ourName, alloc1))
	alloc1.Start()

	doneChan := make(chan bool)
	f := func() {
		_, ok := alloc1.Allocate(container1, subnet, returnFalse)
		doneChan <- ok == nil
	}

	// Attempt two allocations in parallel, to check that this is handled correctly
	go f()
	go f()

	// Nothing should happen, because we declared the quorum as 2
	time.Sleep(100 * time.Millisecond)
	AssertNothingSent(t, doneChan)

	alloc1.ContainerDied(container1)

	// Check that the two allocations both exit with an error
	if <-doneChan || <-doneChan {
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
	alloc.Allocate(container1, subnet, returnFalse)

	alloc.Shutdown()

	_, err := alloc.Allocate(container2, subnet, returnFalse) // trying to allocate after shutdown should fail
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

	_, err := alloc2.Allocate("foo", subnet, returnFalse)
	require.True(t, err == nil, "Failed to get address")

	_, err = alloc3.Allocate("bar", subnet, returnFalse)
	require.True(t, err == nil, "Failed to get address")

	alloc2.gossip.GossipBroadcast(alloc2.Gossip())
	router.Flush()
	alloc2.gossip.GossipBroadcast(alloc3.Gossip())
	router.Flush()
	router.RemovePeer(alloc2.ourName)
	router.RemovePeer(alloc3.ourName)
	alloc2.Stop()
	alloc3.Stop()
	router.Flush()
	require.NoError(t, alloc1.AdminTakeoverRanges(alloc2.ourName.String()))
	require.NoError(t, alloc1.AdminTakeoverRanges(alloc3.ourName.String()))
	router.Flush()

	require.Equal(t, address.Offset(1022), alloc1.NumFreeAddresses(subnet))

	_, err = alloc1.Allocate("foo", subnet, returnFalse)
	require.True(t, err == nil, "Failed to get address")
	alloc1.Stop()
}

func TestFakeRouterSimple(t *testing.T) {
	const (
		cidr = "10.0.1.7/22"
	)
	allocs, _, subnet := makeNetworkOfAllocators(2, cidr)
	defer stopNetworkOfAllocators(allocs)

	alloc1 := allocs[0]
	//alloc2 := allocs[1]

	_, err := alloc1.Allocate("foo", subnet, returnFalse)
	require.NoError(t, err, "Failed to get address")
}

func TestAllocatorFuzz(t *testing.T) {
	const (
		firstpass    = 1000
		secondpass   = 10000
		nodes        = 5
		maxAddresses = 1000
		concurrency  = 5
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

	bumpPending := func() bool {
		stateLock.Lock()
		if len(addrs)+numPending >= maxAddresses {
			stateLock.Unlock()
			return false
		}
		numPending++
		stateLock.Unlock()
		return true
	}

	noteAllocation := func(allocIndex int32, name string, addr address.Address) {
		//common.Log.Infof("Allocate: got address %s for name %s", addr, name)
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

	// Do a Allocate and check the address
	// is unique.  Needs a unique container
	// name.
	allocate := func(name string) {
		if !bumpPending() {
			return
		}

		allocIndex := rand.Int31n(nodes)
		alloc := allocs[allocIndex]
		//common.Log.Infof("Allocate: asking allocator %d", allocIndex)
		addr, err := alloc.Allocate(name, subnet, returnFalse)

		if err != nil {
			panic(fmt.Sprintf("Could not allocate addr"))
		}

		noteAllocation(allocIndex, name, addr)
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
		//common.Log.Infof("Freeing %s (%s) on allocator %d", res.name, addr, res.alloc)

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

		//common.Log.Infof("Asking for %s (%s) on allocator %d again", res.name, addr, res.alloc)

		newAddr, _ := alloc.Allocate(res.name, subnet, returnFalse)
		oldAddr, _ := address.ParseIP(addr)
		if newAddr != oldAddr {
			panic(fmt.Sprintf("Got different address for repeat request for %s: %s != %s", res.name, newAddr, oldAddr))
		}

		stateLock.Lock()
		res.block = false
		state[addr] = res
		stateLock.Unlock()
	}

	// Claim a random address for a unique container name - may not succeed
	claim := func(name string) {
		if !bumpPending() {
			return
		}
		allocIndex := rand.Int31n(nodes)
		addressIndex := rand.Int31n(int32(subnet.Size()))
		alloc := allocs[allocIndex]
		addr := address.Add(subnet.Start, address.Offset(addressIndex))
		err := alloc.Claim(name, addr, true)
		if err == nil {
			noteAllocation(allocIndex, name, addr)
		}
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

		case 0.8 <= r && r < 0.95:
			// ask for an existing name again, check we get same ip
			allocateAgain()

		case 0.95 <= r && r < 1.0:
			name := fmt.Sprintf("second%d", iteration)
			claim(name)
		}
	})
}

func TestGossipSkew(t *testing.T) {
	alloc1, _ := makeAllocatorWithMockGossip(t, "01:00:00:01:00:00", "10.0.1.0/22", 2)
	defer alloc1.Stop()
	alloc2, _ := makeAllocatorWithMockGossip(t, "02:00:00:02:00:00", "10.0.1.0/22", 2)
	alloc2.now = func() time.Time { return time.Now().Add(time.Hour * 2) }
	defer alloc2.Stop()

	if _, err := alloc1.OnGossipBroadcast(alloc2.ourName, alloc2.Encode()); err == nil {
		t.Fail()
	}
}
