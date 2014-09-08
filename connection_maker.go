package weave

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
	MaxAttemptCount = 100
	CMEnsure        = iota
	CMStatus        = iota
	CMEstablished   = iota
	CMConnSucceeded = iota
	CMConnFailed    = iota
)

func StartConnectionMaker(router *Router) *ConnectionMaker {
	queryChan := make(chan *ConnectionMakerInteraction, ChannelSize)
	state := &ConnectionMaker{
		router:    router,
		queryChan: queryChan,
		targets:   make(map[string]*Target)}
	go state.queryLoop(queryChan)
	return state
}

func (cm *ConnectionMaker) InitiateConnection(address string) {
	cm.queryChan <- &ConnectionMakerInteraction{
		Interaction:   Interaction{code: CMEnsure},
		acceptAnyPeer: true,
		address:       address}
}

func (cm *ConnectionMaker) EnsureConnection(address string) {
	// If we've been given a port number, take it off
	if addrHost, _, err := net.SplitHostPort(address); err == nil {
		address = addrHost
	}
	cm.queryChan <- &ConnectionMakerInteraction{
		Interaction:   Interaction{code: CMEnsure},
		acceptAnyPeer: false,
		address:       address}
}

func (cm *ConnectionMaker) ConnectionEstablished(address string) {
	// If we've been given a port number, take it off
	if addrHost, _, err := net.SplitHostPort(address); err == nil {
		address = addrHost
	}
	cm.queryChan <- &ConnectionMakerInteraction{
		Interaction: Interaction{code: CMEstablished},
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
	tickNow := func() {
		if tick == nil {
			tick = time.After(0 * time.Second)
		}
	}
	for {
		select {
		case query, ok := <-queryChan:
			if !ok {
				return
			}
			switch {
			case query.code == CMEnsure:
				cm.addToTargets(query.acceptAnyPeer, query.address)
				cm.resetTarget(query.address)
				tickNow()
			case query.code == CMStatus:
				query.resultChan <- cm.status()
			case query.code == CMEstablished:
				target := cm.targets[query.address]
				if target != nil {
					target.established = true
				} else {
					log.Fatal("Connection established to unknown address", query.address)
				}
			case query.code == CMConnSucceeded:
				cm.targets[query.address].attempting = false
			case query.code == CMConnFailed:
				target := cm.targets[query.address]
				target.attempting = false
				after, interval := tryAfter(target.tryInterval)
				target.tryInterval = interval
				target.tryAfter = after
				maybeTick()
			default:
				log.Fatal("Unexpected connection maker query:", query)
			}
		case now := <-tick:
			for address, target := range cm.targets {
				if !target.established && !target.attempting && now.After(target.tryAfter) {
					target.attemptCount += 1
					target.attempting = true
					go cm.attemptConnection(address, target.acceptAnyPeer)
				}
			}
			tick = nil
			maybeTick()
		}
	}
}

func (cm *ConnectionMaker) addToTargets(acceptAnyPeer bool, address string) {
	target := cm.targets[address]
	if target == nil {
		target = &Target{
			acceptAnyPeer: acceptAnyPeer,
		}
		cm.targets[address] = target
	}
}

func (cm *ConnectionMaker) resetTarget(address string) {
	target := cm.targets[address]
	target.established = false
	after, interval := tryImmediately()
	target.tryInterval = interval
	target.tryAfter = after
}

func (cm *ConnectionMaker) status() string {
	var buf bytes.Buffer
	for address, target := range cm.targets {
		if target.established {
			buf.WriteString(fmt.Sprintf("%s connected\n", address))
		} else if target.attempting {
			buf.WriteString(fmt.Sprintf("%s (%v attempts, trying since %v)\n", address, target.attemptCount, target.tryAfter))
		} else {
			buf.WriteString(fmt.Sprintf("%s (%v attempts, next at %v)\n", address, target.attemptCount, target.tryAfter))
		}
	}
	return buf.String()
}

func (cm *ConnectionMaker) attemptConnection(address string, acceptNewPeer bool) {
	log.Println("Attempting connection to", address)
	var conncode = CMConnSucceeded
	if err := cm.router.Ourself.CreateConnection(address, acceptNewPeer); err != nil {
		log.Println(err)
		conncode = CMConnFailed
	}
	// Tell the query loop we've finished this attempt
	cm.queryChan <- &ConnectionMakerInteraction{
		Interaction: Interaction{code: conncode},
		address:     address}
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
