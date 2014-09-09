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
	CMEnsure        = iota
	CMStatus        = iota
	CMEstablished   = iota
	CMConnFailed    = iota
)

const (
	CSUnconnected ConnectionState = iota
	CSAttempting                  = iota
	CSEstablished                 = iota
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
	address = StripPortFromAddr(address)
	cm.queryChan <- &ConnectionMakerInteraction{
		Interaction:   Interaction{code: CMEnsure},
		acceptAnyPeer: false,
		address:       address}
}

func (cm *ConnectionMaker) ConnectionEstablished(address string) {
	address = StripPortFromAddr(address)
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
				log.Println("Connection ensure", query.address)
				cm.addToTargets(query.acceptAnyPeer, query.address)
				cm.resetTarget(query.address)
				tickNow()
			case query.code == CMStatus:
				query.resultChan <- cm.status()
			case query.code == CMEstablished:
				target := cm.targets[query.address]
				if target != nil {
					log.Println("Connection established to", query.address, target)
					target.state = CSEstablished
				} else {
					log.Fatal("Connection established to unknown address", query.address)
				}
			case query.code == CMConnFailed:
				target := cm.targets[query.address]
				target.state = CSUnconnected
				target.tryAfter, target.tryInterval = tryAfter(target.tryInterval)
				maybeTick()
			default:
				log.Fatal("Unexpected connection maker query:", query)
			}
		case now := <-tick:
			for address, target := range cm.targets {
				if target.state == CSUnconnected && now.After(target.tryAfter) {
					/*if _, found := cm.router.Ourself.ConnectionOn(address); found {
						target.state = CSEstablished
					} else*/ {
						target.attemptCount += 1
						target.state = CSAttempting
						go cm.attemptConnection(address, target.acceptAnyPeer)
					}
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
			state:         CSUnconnected,
			acceptAnyPeer: acceptAnyPeer,
		}
		cm.targets[address] = target
	}
}

func (cm *ConnectionMaker) resetTarget(address string) {
	target := cm.targets[address]
	// If we were in the middle of a connection attempt then back-off again
	if target.state == CSAttempting {
		target.tryAfter, target.tryInterval = tryAfter(target.tryInterval)
	} else {
		target.tryAfter, target.tryInterval = tryImmediately()
	}
	target.state = CSUnconnected
}

func (cm *ConnectionMaker) status() string {
	var buf bytes.Buffer
	for address, target := range cm.targets {
		if target.state == CSEstablished {
			buf.WriteString(fmt.Sprintf("%s connected\n", address))
		} else if target.state == CSAttempting {
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
		cm.queryChan <- &ConnectionMakerInteraction{
			Interaction: Interaction{code: CMConnFailed},
			address:     address}
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
