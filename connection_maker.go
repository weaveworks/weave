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
	CMInitiate = iota
	CMStatus   = iota
	CMFailed   = iota
)

const (
	CSUnconnected ConnectionState = iota
	CSAttempting                  = iota
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

func (cm *ConnectionMaker) ShutdownConnection(address string) {
	cm.queryChan <- &ConnectionMakerInteraction{
		Interaction: Interaction{code: CMFailed},
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
			case query.code == CMFailed:
				if target, found := cm.targets[query.address]; found {
					target.state = CSUnconnected
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
		//log.Println("Connected peer:", peer, conn.RemoteTCPAddr())
		our_connected_peers[peer] = true
		our_connected_targets[conn.RemoteTCPAddr()] = true
	})

	// Add command-line targets that are not connected
	for address, _ := range cm.cmdLineAddress {
		if !our_connected_targets[NormalisePeerAddr(address)] {
			//log.Println("Unconnected cmdline:", address)
			validTarget[address] = true
		}
	}

	// Add peers that someone else is connected to, but we aren't
	cm.router.Peers.ForEach(func(name PeerName, peer *Peer) {
		peer.ForEachConnection(func(peer2 PeerName, conn Connection) {
			if peer2 != ourself.Name && !our_connected_peers[peer2] {
				address := conn.RemoteTCPAddr()
				//log.Println("Unconnected peer:", peer2, address)
				// try both portnumber of connection and standart port
				if host, port, err := ExtractHostPort(address); err == nil {
					if port != Port {
						// This is an address with port, that is not the standard port
						validTarget[address] = true
					}
					// Add the address with standard port
					validTarget[NormalisePeerAddr(host)] = true
				}
			}
		})
	})

	for address, _ := range validTarget {
		cm.addToTargets(address)
	}

	now = time.Now() // make sure we catch items just added
	for address, target := range cm.targets {
		if target.state == CSUnconnected {
			if our_connected_targets[address] || !validTarget[address] {
				//log.Println("Deleting target no longer valid:", address)
				delete(cm.targets, address)
			} else if now.After(target.tryAfter) {
				target.attemptCount += 1
				target.state = CSAttempting
				go cm.attemptConnection(address, cm.cmdLineAddress[address])
			}
		} else if our_connected_targets[address] {
			//log.Println("Deleting target now connected:", address)
			delete(cm.targets, address)
		}
	}
}

func (cm *ConnectionMaker) addToTargets(address string) {
	address = NormalisePeerAddr(address)
	target := cm.targets[address]
	if target == nil {
		target = &Target{
			state: CSUnconnected,
		}
		target.tryAfter, target.tryInterval = tryImmediately()
		cm.targets[address] = target
	}
}

func (cm *ConnectionMaker) status() string {
	var buf bytes.Buffer
	for address, target := range cm.targets {
		if target.state == CSAttempting {
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
		cm.ShutdownConnection(address)
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
