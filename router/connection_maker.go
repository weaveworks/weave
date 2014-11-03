package router

import (
	"bytes"
	"fmt"
	"log"
	"math/rand"
	"net"
	"time"
)

const (
	InitialInterval = 5 * time.Second
	MaxInterval     = 10 * time.Minute
)

const (
	CMInitiate   = iota
	CMTerminated = iota
	CMRefresh    = iota
	CMStatus     = iota
)

func StartConnectionMaker(router *Router) *ConnectionMaker {
	queryChan := make(chan *ConnectionMakerInteraction, ChannelSize)
	state := &ConnectionMaker{
		router:         router,
		queryChan:      queryChan,
		cmdLineAddress: make(map[string]bool),
		targets:        make(map[string]*Target)}
	go state.queryLoop(queryChan)
	return state
}

func (cm *ConnectionMaker) InitiateConnection(address string) {
	cm.queryChan <- &ConnectionMakerInteraction{
		Interaction: Interaction{code: CMInitiate},
		address:     address}
}

func (cm *ConnectionMaker) ConnectionTerminated(address string) {
	cm.queryChan <- &ConnectionMakerInteraction{
		Interaction: Interaction{code: CMTerminated},
		address:     address}
}

func (cm *ConnectionMaker) Refresh() {
	cm.queryChan <- &ConnectionMakerInteraction{
		Interaction: Interaction{code: CMRefresh}}
}

func (cm *ConnectionMaker) String() string {
	resultChan := make(chan interface{}, 0)
	cm.queryChan <- &ConnectionMakerInteraction{
		Interaction: Interaction{code: CMStatus, resultChan: resultChan}}
	result := <-resultChan
	return result.(string)
}

func (cm *ConnectionMaker) queryLoop(queryChan <-chan *ConnectionMakerInteraction) {
	timer := time.NewTimer(MaxDuration)
	run := func() { timer.Reset(cm.checkStateAndAttemptConnections()) }
	for {
		select {
		case query, ok := <-queryChan:
			if !ok {
				return
			}
			switch query.code {
			case CMInitiate:
				cm.cmdLineAddress[NormalisePeerAddr(query.address)] = true
				run()
			case CMTerminated:
				if target, found := cm.targets[query.address]; found {
					target.attempting = false
					target.tryAfter, target.tryInterval = tryAfter(target.tryInterval)
				}
				run()
			case CMRefresh:
				run()
			case CMStatus:
				run()
				query.resultChan <- cm.status()
			default:
				log.Fatal("Unexpected connection maker query:", query)
			}
		case <-timer.C:
			run()
		}
	}
}

func (cm *ConnectionMaker) checkStateAndAttemptConnections() time.Duration {
	ourself := cm.router.Ourself
	validTarget := make(map[string]bool)

	// copy the set of things we are connected to, so we can access them without locking
	our_connected_peers := make(map[PeerName]bool)
	our_connected_targets := make(map[string]bool)
	ourself.ForEachConnection(func(peer PeerName, conn Connection) {
		our_connected_peers[peer] = true
		our_connected_targets[conn.RemoteTCPAddr()] = true
	})

	addTarget := func(address string) {
		if !our_connected_targets[address] {
			validTarget[address] = true
			cm.addTarget(address)
		}
	}

	// Add command-line targets that are not connected
	for address, _ := range cm.cmdLineAddress {
		addTarget(address)
	}

	// Add targets for peers that someone else is connected to, but we
	// aren't
	cm.router.Peers.ForEach(func(name PeerName, peer *Peer) {
		peer.ForEachConnection(func(otherPeer PeerName, conn Connection) {
			if otherPeer == ourself.Name || our_connected_peers[otherPeer] {
				return
			}
			address := conn.RemoteTCPAddr()
			// try both portnumber of connection and standard port
			addTarget(address)
			if host, _, err := net.SplitHostPort(address); err == nil {
				addTarget(NormalisePeerAddr(host))
			}
		})
	})

	now := time.Now() // make sure we catch items just added
	after := MaxDuration
	for address, target := range cm.targets {
		if our_connected_targets[address] {
			delete(cm.targets, address)
			continue
		}
		if target.attempting {
			continue
		}
		if !validTarget[address] {
			delete(cm.targets, address)
			continue
		}
		switch duration := target.tryAfter.Sub(now); {
		case duration <= 0:
			target.attempting = true
			go cm.attemptConnection(address, cm.cmdLineAddress[address])
		case duration < after:
			after = duration
		}
	}
	return after
}

func (cm *ConnectionMaker) addTarget(address string) {
	if _, found := cm.targets[address]; !found {
		target := &Target{}
		target.tryAfter, target.tryInterval = tryImmediately()
		cm.targets[address] = target
	}
}

func (cm *ConnectionMaker) status() string {
	var buf bytes.Buffer
	for address, target := range cm.targets {
		var fmtStr string
		if target.attempting {
			fmtStr = "%s (trying since %v)\n"
		} else {
			fmtStr = "%s (next try at %v)\n"
		}
		buf.WriteString(fmt.Sprintf(fmtStr, address, target.tryAfter))
	}
	return buf.String()
}

func (cm *ConnectionMaker) attemptConnection(address string, acceptNewPeer bool) {
	log.Println("Attempting connection to", address)
	if err := cm.router.Ourself.CreateConnection(address, acceptNewPeer); err != nil {
		log.Println(err)
		cm.ConnectionTerminated(address)
	}
}

func tryImmediately() (time.Time, time.Duration) {
	interval := time.Duration(rand.Int63n(int64(InitialInterval)))
	return time.Now(), interval
}

func tryAfter(interval time.Duration) (time.Time, time.Duration) {
	interval += time.Duration(rand.Int63n(int64(interval)))
	if interval > MaxInterval {
		interval = MaxInterval
	}
	return time.Now().Add(interval), interval
}
