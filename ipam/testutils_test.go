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

type mockMessage struct {
	dst     router.PeerName
	msgType byte
	buf     []byte
}

func (m *mockMessage) String() string {
	return fmt.Sprintf("-> %s [%x]", m.dst, m.buf)
}

func toStringArray(messages []mockMessage) []string {
	out := make([]string, len(messages))
	for i := range out {
		out[i] = messages[i].String()
	}
	return out
}

type mockGossipComms struct {
	sync.RWMutex
	*testing.T
	name     string
	messages []mockMessage
}

func (m *mockGossipComms) String() string {
	m.RLock()
	defer m.RUnlock()
	return fmt.Sprintf("[mockGossipComms %s]", m.name)
}

// Note: this style of verification, using equalByteBuffer, requires
// that the contents of messages are never re-ordered.  Which, for instance,
// requires they are not based off iterating through a map.

func (m *mockGossipComms) GossipBroadcast(update router.GossipData) error {
	m.Lock()
	defer m.Unlock()
	buf := []byte{}
	if len(m.messages) == 0 {
		require.FailNow(m, fmt.Sprintf("%s: Gossip broadcast message unexpected: \n%x", m.name, buf))
	} else if msg := m.messages[0]; msg.dst != router.UnknownPeerName {
		require.FailNow(m, fmt.Sprintf("%s: Expected Gossip message to %s but got broadcast", m.name, msg.dst))
	} else if msg.buf != nil && !equalByteBuffer(msg.buf, buf) {
		require.FailNow(m, fmt.Sprintf("%s: Gossip message not sent as expected: \nwant: %x\ngot : %x", m.name, msg.buf, buf))
	} else {
		// Swallow this message
		m.messages = m.messages[1:]
	}
	return nil
}

func equalByteBuffer(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (m *mockGossipComms) GossipUnicast(dstPeerName router.PeerName, buf []byte) error {
	m.Lock()
	defer m.Unlock()
	if len(m.messages) == 0 {
		require.FailNow(m, fmt.Sprintf("%s: Gossip message to %s unexpected: \n%s", m.name, dstPeerName, buf))
	} else if msg := m.messages[0]; msg.dst == router.UnknownPeerName {
		require.FailNow(m, fmt.Sprintf("%s: Expected Gossip broadcast message but got dest %s", m.name, dstPeerName))
	} else if msg.dst != dstPeerName {
		require.FailNow(m, fmt.Sprintf("%s: Expected Gossip message to %s but got dest %s", m.name, msg.dst, dstPeerName))
	} else if buf[0] != msg.msgType {
		require.FailNow(m, fmt.Sprintf("%s: Expected Gossip message of type %d but got type %d", m.name, msg.msgType, buf[0]))
	} else if msg.buf != nil && !equalByteBuffer(msg.buf, buf[1:]) {
		require.FailNow(m, fmt.Sprintf("%s: Gossip message not sent as expected: \nwant: %x\ngot : %x", m.name, msg.buf, buf[1:]))
	} else {
		// Swallow this message
		m.messages = m.messages[1:]
	}
	return nil
}

func ExpectMessage(alloc *Allocator, dst string, msgType byte, buf []byte) {
	m := alloc.gossip.(*mockGossipComms)
	dstPeerName, _ := router.PeerNameFromString(dst)
	m.Lock()
	m.messages = append(m.messages, mockMessage{dstPeerName, msgType, buf})
	m.Unlock()
}

func ExpectBroadcastMessage(alloc *Allocator, buf []byte) {
	m := alloc.gossip.(*mockGossipComms)
	m.Lock()
	m.messages = append(m.messages, mockMessage{router.UnknownPeerName, 0, buf})
	m.Unlock()
}

func CheckAllExpectedMessagesSent(allocs ...*Allocator) {
	for _, alloc := range allocs {
		m := alloc.gossip.(*mockGossipComms)
		m.RLock()
		if len(m.messages) > 0 {
			require.FailNow(m, fmt.Sprintf("%s: Gossip message(s) not sent as expected: \n%x", m.name, m.messages))
		}
		m.RUnlock()
	}
}

func makeAllocator(name string, cidrStr string, quorum uint) (*Allocator, address.Range) {
	peername, err := router.PeerNameFromString(name)
	if err != nil {
		panic(err)
	}

	_, cidr, err := address.ParseCIDR(cidrStr)
	if err != nil {
		panic(err)
	}

	alloc := NewAllocator(peername, router.PeerUID(rand.Int63()),
		"nick-"+name, cidr.Range(), quorum)

	return alloc, cidr.HostRange()
}

func makeAllocatorWithMockGossip(t *testing.T, name string, universeCIDR string, quorum uint) (*Allocator, address.Range) {
	alloc, subnet := makeAllocator(name, universeCIDR, quorum)
	gossip := &mockGossipComms{T: t, name: name}
	alloc.SetInterfaces(gossip)
	alloc.Start()
	return alloc, subnet
}

func (alloc *Allocator) claimRingForTesting(allocs ...*Allocator) {
	peers := []router.PeerName{alloc.ourName}
	for _, alloc2 := range allocs {
		peers = append(peers, alloc2.ourName)
	}
	alloc.ring.ClaimForPeers(normalizeConsensus(peers))
	alloc.space.AddRanges(alloc.ring.OwnedRanges())
}

func (alloc *Allocator) NumFreeAddresses(r address.Range) address.Offset {
	resultChan := make(chan address.Offset)
	alloc.actionChan <- func() {
		resultChan <- alloc.space.NumFreeAddressesInRange(r)
	}
	return <-resultChan
}

// Check whether or not something was sent on a channel
func AssertSent(t *testing.T, ch <-chan bool) {
	timeout := time.After(10 * time.Second)
	select {
	case <-ch:
		// This case is ok
	case <-timeout:
		require.FailNow(t, "Nothing sent on channel")
	}
}

func AssertNothingSent(t *testing.T, ch <-chan bool) {
	select {
	case val := <-ch:
		require.FailNow(t, fmt.Sprintf("Unexpected value on channel: %v", val))
	default:
		// no message received
	}
}

func AssertNothingSentErr(t *testing.T, ch <-chan error) {
	select {
	case val := <-ch:
		require.FailNow(t, fmt.Sprintf("Unexpected value on channel: %v", val))
	default:
		// no message received
	}
}

// Router to convey gossip from one gossiper to another, for testing
type unicastMessage struct {
	sender router.PeerName
	buf    []byte
}
type broadcastMessage struct {
	data router.GossipData
}
type exitMessage struct {
	exitChan chan struct{}
}
type flushMessage struct {
	flushChan chan struct{}
}

type TestGossipRouter struct {
	gossipChans map[router.PeerName]chan interface{}
	loss        float32 // 0.0 means no loss
}

func (grouter *TestGossipRouter) GossipBroadcast(update router.GossipData) error {
	for _, gossipChan := range grouter.gossipChans {
		select {
		case gossipChan <- broadcastMessage{data: update}:
		default: // drop the message if we cannot send it
		}
	}
	return nil
}

func (grouter *TestGossipRouter) flush() {
	for _, gossipChan := range grouter.gossipChans {
		flushChan := make(chan struct{})
		gossipChan <- flushMessage{flushChan: flushChan}
		<-flushChan
	}
}

func (grouter *TestGossipRouter) removePeer(peer router.PeerName) {
	gossipChan := grouter.gossipChans[peer]
	resultChan := make(chan struct{})
	gossipChan <- exitMessage{exitChan: resultChan}
	<-resultChan
	delete(grouter.gossipChans, peer)
}

type TestGossipRouterClient struct {
	router *TestGossipRouter
	sender router.PeerName
}

func (grouter *TestGossipRouter) run(gossiper router.Gossiper, gossipChan chan interface{}) {
	gossipTimer := time.Tick(10 * time.Second)
	for {
		select {
		case gossip := <-gossipChan:
			switch message := gossip.(type) {
			case exitMessage:
				close(message.exitChan)
				return

			case flushMessage:
				close(message.flushChan)

			case unicastMessage:
				if rand.Float32() > (1.0 - grouter.loss) {
					continue
				}
				if err := gossiper.OnGossipUnicast(message.sender, message.buf); err != nil {
					panic(fmt.Sprintf("Error doing gossip unicast to %s: %s", message.sender, err))
				}

			case broadcastMessage:
				if rand.Float32() > (1.0 - grouter.loss) {
					continue
				}
				for _, msg := range message.data.Encode() {
					if _, err := gossiper.OnGossipBroadcast(msg); err != nil {
						panic(fmt.Sprintf("Error doing gossip broadcast: %s", err))
					}
				}
			}
		case <-gossipTimer:
			grouter.GossipBroadcast(gossiper.Gossip())
		}
	}
}

func (grouter *TestGossipRouter) connect(sender router.PeerName, gossiper router.Gossiper) router.Gossip {
	gossipChan := make(chan interface{}, 100)

	go grouter.run(gossiper, gossipChan)

	grouter.gossipChans[sender] = gossipChan
	return TestGossipRouterClient{grouter, sender}
}

func (client TestGossipRouterClient) GossipUnicast(dstPeerName router.PeerName, buf []byte) error {
	select {
	case client.router.gossipChans[dstPeerName] <- unicastMessage{sender: client.sender, buf: buf}:
	default: // drop the message if we cannot send it
		common.Error.Printf("Dropping message")
	}
	return nil
}

func (client TestGossipRouterClient) GossipBroadcast(update router.GossipData) error {
	return client.router.GossipBroadcast(update)
}

func makeNetworkOfAllocators(size int, cidr string) ([]*Allocator, TestGossipRouter, address.Range) {

	gossipRouter := TestGossipRouter{make(map[router.PeerName]chan interface{}), 0.0}
	allocs := make([]*Allocator, size)
	var subnet address.Range

	for i := 0; i < size; i++ {
		var alloc *Allocator
		alloc, subnet = makeAllocator(fmt.Sprintf("%02d:00:00:02:00:00", i),
			cidr, uint(size/2+1))
		alloc.SetInterfaces(gossipRouter.connect(alloc.ourName, alloc))
		alloc.Start()
		allocs[i] = alloc
	}

	gossipRouter.GossipBroadcast(allocs[size-1].Gossip())
	gossipRouter.flush()
	return allocs, gossipRouter, subnet
}

func stopNetworkOfAllocators(allocs []*Allocator) {
	for _, alloc := range allocs {
		alloc.Stop()
	}
}
