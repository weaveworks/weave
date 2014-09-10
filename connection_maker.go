package weave

import (
	"bytes"
	"fmt"
	"log"
	"math/rand"
	"time"
)

const (
	InitialInterval = 5 * time.Second
	MaxInterval     = 10 * time.Minute
	MaxAttemptCount = 100
)

const (
	CMInitiate   = iota
	CMStatus     = iota
	CMTerminated = iota
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

func (cm *ConnectionMaker) String() string {
	resultChan := make(chan interface{}, 0)
	cm.queryChan <- &ConnectionMakerInteraction{
		Interaction: Interaction{code: CMStatus, resultChan: resultChan}}
	result := <-resultChan
	return result.(string)
}

func (cm *ConnectionMaker) queryLoop(queryChan <-chan *ConnectionMakerInteraction) {
	var tick <-chan time.Time
	maybeTick := func() {
		// would be nice to optimise this to stop ticking when there is nothing worth trying
		if tick == nil {
			tick = time.After(5 * time.Second)
		}
	}
	for {
		select {
		case query, ok := <-queryChan:
			if !ok {
				return
			}
			switch {
			case query.code == CMInitiate:
				cm.cmdLineAddress[NormalisePeerAddr(query.address)] = true
				cm.checkStateAndAttemptConnections(time.Now())
				maybeTick()
			case query.code == CMStatus:
				query.resultChan <- cm.status()
			case query.code == CMTerminated:
				if target, found := cm.targets[query.address]; found {
					target.attempting = false
					target.tryAfter, target.tryInterval = tryAfter(target.tryInterval)
					maybeTick()
				}
			default:
				log.Fatal("Unexpected connection maker query:", query)
			}
		case now := <-tick:
			cm.checkStateAndAttemptConnections(now)
			tick = nil
			maybeTick()
		}
	}
}

func (cm *ConnectionMaker) checkStateAndAttemptConnections(now time.Time) {
	ourself := cm.router.Ourself
	validTarget := make(map[string]bool)

	// copy the set of things we are connected to, so we can access them without locking
	our_connected_peers := make(map[PeerName]bool)
	our_connected_targets := make(map[string]bool)
	ourself.ForEachConnection(func(peer PeerName, conn Connection) {
		our_connected_peers[peer] = true
		our_connected_targets[conn.RemoteTCPAddr()] = true
	})

	// Add command-line targets that are not connected
	for address, _ := range cm.cmdLineAddress {
		if !our_connected_targets[NormalisePeerAddr(address)] {
			validTarget[address] = true
		}
	}

	// Add peers that someone else is connected to, but we aren't
	cm.router.Peers.ForEach(func(name PeerName, peer *Peer) {
		peer.ForEachConnection(func(peer2 PeerName, conn Connection) {
			if peer2 == ourself.Name || our_connected_peers[peer2] {
				return
			}
			address := conn.RemoteTCPAddr()
			// try both portnumber of connection and standart port
			validTarget[address] = true
			if host, _, err := ExtractHostPort(address); err == nil {
				validTarget[NormalisePeerAddr(host)] = true
			}
		})
	})

	for address, _ := range validTarget {
		cm.addToTargets(address)
	}

	now = time.Now() // make sure we catch items just added
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
		if now.After(target.tryAfter) {
			target.attempting = true
			target.attemptCount += 1
			go cm.attemptConnection(address, cm.cmdLineAddress[address])
		}
	}
}

func (cm *ConnectionMaker) addToTargets(address string) {
	address = NormalisePeerAddr(address)
	target := cm.targets[address]
	if target == nil {
		target = &Target{}
		target.tryAfter, target.tryInterval = tryImmediately()
		cm.targets[address] = target
	}
}

func (cm *ConnectionMaker) status() string {
	var buf bytes.Buffer
	for address, target := range cm.targets {
		if target.attempting {
			buf.WriteString(fmt.Sprintf("%s (%v attempts, trying since %v)\n", address, target.attemptCount, target.tryAfter))
		} else {
			buf.WriteString(fmt.Sprintf("%s (%v attempts, next at %v)\n", address, target.attemptCount, target.tryAfter))
		}
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
