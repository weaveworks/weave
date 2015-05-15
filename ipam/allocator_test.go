package ipam

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/ipam/address"
	"github.com/weaveworks/weave/router"
	wt "github.com/weaveworks/weave/testing"
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
		universe   = "10.0.3.0/28"
		testAddr1  = "10.0.3.1" // first address allocated should be .1 because .0 is network addr
		spaceSize  = 14         // 16 IP addresses in /28, minus .0 and .15
	)

	alloc := makeAllocatorWithMockGossip(t, "01:00:00:01:00:00", universe, 1)
	defer alloc.Stop()

	alloc.claimRingForTesting()
	addr1, _ := alloc.Allocate(container1, nil)
	wt.AssertEqualString(t, addr1.String(), testAddr1, "address")

	// Ask for another address for a different container and check it's different
	addr2, _ := alloc.Allocate(container2, nil)
	if addr2.String() == testAddr1 {
		t.Fatalf("Expected different address but got %s", addr2.String())
	}

	// Ask for the first container again and we should get the same address again
	addr1a, _ := alloc.Allocate(container1, nil)
	wt.AssertEqualString(t, addr1a.String(), testAddr1, "address")

	// Now free the first one, and we should get it back when we ask
	wt.AssertSuccess(t, alloc.Free(container1))
	addr3, _ := alloc.Allocate(container3, nil)
	wt.AssertEqualString(t, addr3.String(), testAddr1, "address")

	alloc.ContainerDied(container2)
	alloc.ContainerDied(container3)
	alloc.String() // force sync-up after async call
	wt.AssertEquals(t, alloc.space.NumFreeAddresses(), address.Offset(spaceSize))
}

func TestBootstrap(t *testing.T) {
	common.InitDefaultLogging(false)
	const (
		donateSize     = 5
		donateStart    = "10.0.1.7"
		ourNameString  = "01:00:00:01:00:00"
		peerNameString = "02:00:00:02:00:00"
	)

	alloc1 := makeAllocatorWithMockGossip(t, ourNameString, testStart1+"/22", 2)
	defer alloc1.Stop()

	// Simulate another peer on the gossip network
	alloc2 := makeAllocatorWithMockGossip(t, peerNameString, testStart1+"/22", 2)
	defer alloc2.Stop()

	alloc1.OnGossipBroadcast(alloc2.encode())

	alloc1.tryPendingOps()

	ExpectBroadcastMessage(alloc1, nil) // alloc1 will try to form consensus
	done := make(chan bool)
	go func() {
		alloc1.Allocate("somecontainer", nil)
		done <- true
	}()
	time.Sleep(100 * time.Millisecond)
	AssertNothingSent(t, done)

	CheckAllExpectedMessagesSent(alloc1, alloc2)

	alloc1.tryPendingOps()
	AssertNothingSent(t, done)

	CheckAllExpectedMessagesSent(alloc1, alloc2)

	// alloc2 receives paxos update and broadcasts its reply
	ExpectBroadcastMessage(alloc2, nil)
	alloc2.OnGossipBroadcast(alloc1.encode())

	ExpectBroadcastMessage(alloc1, nil)
	alloc1.OnGossipBroadcast(alloc2.encode())

	// both nodes will get consensus now so initialize the ring
	ExpectBroadcastMessage(alloc2, nil)
	ExpectBroadcastMessage(alloc2, nil)
	alloc2.OnGossipBroadcast(alloc1.encode())

	CheckAllExpectedMessagesSent(alloc1, alloc2)

	alloc1.OnGossipBroadcast(alloc2.encode())
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

	alloc := makeAllocatorWithMockGossip(t, "01:00:00:01:00:00", universe, 1)
	defer alloc.Stop()

	alloc.claimRingForTesting()
	addr1, _ := alloc.Allocate(container1, nil)
	alloc.Allocate(container2, nil)

	// Now free the first one, and try to claim it
	wt.AssertSuccess(t, alloc.Free(container1))
	t.Log(alloc)
	err := alloc.Claim(container3, addr1, nil)
	wt.AssertNoErr(t, err)
	addr3, _ := alloc.Allocate(container3, nil)
	wt.AssertEqualString(t, addr3.String(), testAddr1, "address")
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

	router := TestGossipRouter{make(map[router.PeerName]chan gossipMessage), 0.0}

	alloc1 := makeAllocator("01:00:00:02:00:00", CIDR, 2)
	alloc1.SetInterfaces(router.connect(alloc1.ourName, alloc1))

	alloc2 := makeAllocator("02:00:00:02:00:00", CIDR, 2)
	alloc2.SetInterfaces(router.connect(alloc2.ourName, alloc2))
	alloc1.claimRingForTesting(alloc1, alloc2)
	alloc2.claimRingForTesting(alloc1, alloc2)

	alloc1.Start()
	alloc2.Start()

	// tell peers about each other
	alloc1.OnGossipBroadcast(alloc2.encode())

	// Get some IPs, so each allocator has some space
	res1, _ := alloc1.Allocate("foo", nil)
	common.Debug.Printf("res1 = %s", res1.String())
	res2, _ := alloc2.Allocate("bar", nil)
	common.Debug.Printf("res2 = %s", res2.String())
	if res1 == res2 {
		wt.Fatalf(t, "Error: got same ips!")
	}

	// Now we're going to pause alloc2 and ask alloc1
	// for an allocation
	unpause := alloc2.pause()

	// Use up all the IPs that alloc1 owns, so the allocation after this will prompt a request to alloc2
	for i := 0; alloc1.space.NumFreeAddresses() > 0; i++ {
		alloc1.Allocate(fmt.Sprintf("tmp%d", i), nil)
	}
	cancelChan := make(chan bool, 1)
	doneChan := make(chan bool)
	go func() {
		_, ok := alloc1.Allocate("baz", cancelChan)
		doneChan <- ok == nil
	}()

	AssertNothingSent(t, doneChan)
	time.Sleep(100 * time.Millisecond)
	AssertNothingSent(t, doneChan)

	cancelChan <- true
	unpause()
	if <-doneChan {
		wt.Fatalf(t, "Error: got result from Allocate")
	}
}

func TestGossipShutdown(t *testing.T) {
	const (
		container1 = "abcdef"
		container2 = "baddf00d"
		universe   = "10.0.3.0/30"
		testAddr1  = "10.0.3.1" // first address allocated should be .1 because .0 is network addr
	)

	alloc := makeAllocatorWithMockGossip(t, "01:00:00:01:00:00", universe, 1)
	defer alloc.Stop()

	alloc.claimRingForTesting()
	addr1, _ := alloc.Allocate(container1, nil)
	wt.AssertEqualString(t, addr1.String(), testAddr1, "address")

	alloc.Shutdown()

	_, err := alloc.Allocate(container2, nil) // trying to allocate after shutdown should fail
	wt.AssertFalse(t, err == nil, "no address")

	CheckAllExpectedMessagesSent(alloc)
}

func TestTransfer(t *testing.T) {
	const (
		cidr = "10.0.1.7/22"
	)
	allocs, router := makeNetworkOfAllocators(3, cidr)
	alloc1 := allocs[0]
	alloc2 := allocs[1]
	alloc3 := allocs[2] // This will be 'master' and get the first range

	_, err := alloc2.Allocate("foo", nil)
	wt.AssertTrue(t, err == nil, "Failed to get address")

	_, err = alloc3.Allocate("bar", nil)
	wt.AssertTrue(t, err == nil, "Failed to get address")

	router.GossipBroadcast(alloc2.Gossip())
	router.GossipBroadcast(alloc3.Gossip())
	router.removePeer(alloc2.ourName)
	router.removePeer(alloc3.ourName)
	alloc2.Stop()
	alloc3.Stop()
	wt.AssertSuccess(t, alloc1.AdminTakeoverRanges(alloc2.ourName.String()))
	wt.AssertSuccess(t, alloc1.AdminTakeoverRanges(alloc3.ourName.String()))

	wt.AssertEquals(t, alloc1.space.NumFreeAddresses(), address.Offset(1022))

	_, err = alloc1.Allocate("foo", nil)
	wt.AssertTrue(t, err == nil, "Failed to get address")
	alloc1.Stop()
}

func TestFakeRouterSimple(t *testing.T) {
	common.InitDefaultLogging(false)
	const (
		cidr = "10.0.1.7/22"
	)
	allocs, _ := makeNetworkOfAllocators(2, cidr)
	defer stopNetworkOfAllocators(allocs)

	alloc1 := allocs[0]
	//alloc2 := allocs[1]

	addr, _ := alloc1.Allocate("foo", nil)
	println("Got addr", addr)
}

func TestAllocatorFuzz(t *testing.T) {
	common.InitDefaultLogging(false)
	const (
		firstpass    = 1000
		secondpass   = 5000
		nodes        = 10
		maxAddresses = 1000
		concurrency  = 10
		cidr         = "10.0.1.7/22"
	)
	allocs, _ := makeNetworkOfAllocators(nodes, cidr)
	defer stopNetworkOfAllocators(allocs)

	// Test state
	// For each IP issued we store the allocator
	// that issued it and the name of the container
	// it was issued to.
	type result struct {
		name  string
		alloc int32
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
		addr, err := alloc.Allocate(name, nil)

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

		state[addrStr] = result{name, allocIndex}
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
		addrs = rm(addrs, addrIndex)
		delete(state, addr)
		stateLock.Unlock()

		alloc := allocs[res.alloc]
		//common.Info.Printf("Freeing %s on allocator %d", addr, res.alloc)

		wt.AssertSuccess(t, alloc.Free(res.name))
	}

	// Do a Allocate on an existing container & allocator
	// and check we get the right answer.
	allocateAgain := func() {
		stateLock.Lock()
		addrIndex := rand.Int31n(int32(len(addrs)))
		addr := addrs[addrIndex]
		res := state[addr]
		stateLock.Unlock()
		alloc := allocs[res.alloc]

		//common.Info.Printf("Asking for %s on allocator %d again", addr, res.alloc)

		newAddr, _ := alloc.Allocate(res.name, nil)
		oldAddr, _ := address.ParseIP(addr)
		if newAddr != oldAddr {
			panic(fmt.Sprintf("Got different address for repeat request"))
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

		case 0.8 <= r && r < 1.0:
			// ask for an existing name again, check we get same ip
			allocateAgain()
		}
	})
}

func TestGossipSkew(t *testing.T) {
	alloc1 := makeAllocatorWithMockGossip(t, "01:00:00:01:00:00", "10.0.1.0/22", 2)
	defer alloc1.Stop()
	alloc2 := makeAllocatorWithMockGossip(t, "02:00:00:02:00:00", "10.0.1.0/22", 2)
	alloc2.now = func() time.Time { return time.Now().Add(time.Hour * 2) }
	defer alloc2.Stop()

	if _, err := alloc1.OnGossipBroadcast(alloc2.encode()); err == nil {
		t.Fail()
	}
}
