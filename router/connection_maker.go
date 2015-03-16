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
	CMInitiate = iota
	CMTerminated
	CMRefresh
	CMStatus
)

type ConnectionMaker struct {
	ourself        *LocalPeer
	peers          *Peers
	targets        map[string]*Target
	cmdLineAddress map[string]bool
	queryChan      chan<- *ConnectionMakerInteraction
}

// Information about an address where we may find a peer
type Target struct {
	attempting  bool          // are we currently attempting to connect there?
	tryAfter    time.Time     // next time to try this address
	tryInterval time.Duration // backoff time on next failure
}

type ConnectionMakerInteraction struct {
	Interaction
	address string
}

func NewConnectionMaker(ourself *LocalPeer, peers *Peers) *ConnectionMaker {
	return &ConnectionMaker{
		ourself:        ourself,
		peers:          peers,
		cmdLineAddress: make(map[string]bool),
		targets:        make(map[string]*Target)}
}

func (cm *ConnectionMaker) Start() {
	queryChan := make(chan *ConnectionMakerInteraction, ChannelSize)
	cm.queryChan = queryChan
	go cm.queryLoop(queryChan)
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
	validTarget := make(map[string]bool)

	// copy the set of things we are connected to, so we can access them without locking
	ourConnectedPeers := make(map[PeerName]bool)
	ourConnectedTargets := make(map[string]bool)
	cm.ourself.ForEachConnection(func(peer PeerName, conn Connection) {
		ourConnectedPeers[peer] = true
		ourConnectedTargets[conn.RemoteTCPAddr()] = true
	})

	addTarget := func(address string) {
		if !ourConnectedTargets[address] {
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
	cm.peers.ForEach(func(name PeerName, peer *Peer) {
		peer.ForEachConnection(func(otherPeer PeerName, conn Connection) {
			if otherPeer == cm.ourself.Name || ourConnectedPeers[otherPeer] {
				return
			}
			address := conn.RemoteTCPAddr()
			// try both portnumber of connection and standard port.  Don't use remote side of inbound connection.
			if conn.Outbound() {
				addTarget(address)
			}
			if host, _, err := net.SplitHostPort(address); err == nil {
				addTarget(NormalisePeerAddr(host))
			}
		})
	})

	now := time.Now() // make sure we catch items just added
	after := MaxDuration
	for address, target := range cm.targets {
		if ourConnectedTargets[address] {
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
	log.Printf("->[%s] attempting connection\n", address)
	if err := cm.ourself.CreateConnection(address, acceptNewPeer); err != nil {
		log.Printf("->[%s] error during connection attempt: %v\n", address, err)
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
