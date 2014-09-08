package weave

/* ConnectionMaker is responsible for making connections between peers,
and retrying them when they break.  It sits in a loop waiting for requests
to be sent over queryChan, or for timer ticks.
Every connection has a PeerName, which is a unique identifier for every weave
router (e.g.  a MAC address), and an IP address which it was foundAt.

*/

import (
	"bytes"
	"fmt"
	"log"
	"math/rand"
	"net"
	"strconv"
	"time"
)

const (
	InitialInterval = 1 * time.Second
	MaxInterval     = 10 * time.Minute
	MaxAttemptCount = 100
	CMEnsure        = iota
	CMStatus        = iota
	CMConnSucceeded = iota
	CMConnFailed    = iota
)

func StartConnectionMaker(router *Router) *ConnectionMaker {
	queryChan := make(chan *ConnectionMakerInteraction, ChannelSize)
	state := &ConnectionMaker{
		router:            router,
		queryChan:         queryChan,
		targets: 		   make(map[string]*Target)}
	go state.queryLoop(queryChan)
	return state
}

func (cm *ConnectionMaker) EnsureConnection(name PeerName, foundAt string) {
	cm.queryChan <- &ConnectionMakerInteraction{
		Interaction: Interaction{code: CMEnsure},
		name:        name,
		foundAt:     foundAt}
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
			case query.code == CMEnsure:
				if query.name != cm.router.Ourself.Name {
					cm.addToTargets(query.name, query.foundAt)
					maybeTick()
				}
			case query.code == CMStatus:
				query.resultChan <- cm.status()
			case query.code == CMConnSucceeded:
				cm.targets[query.foundAt].attempting = false
			case query.code == CMConnFailed:
				cm.targets[query.foundAt].attempting = false
			default:
				log.Fatal("Unexpected connection maker query:", query)
			}
		case now := <-tick:
			for address, target := range cm.targets {
				if target.conn == nil && !target.attempting && now.After(target.tryAfter) {
				    if target.attemptCount == MaxAttemptCount {
					    // FIXME
						continue
					}
					after, interval := tryAfter(target.tryInterval)
					target.tryInterval = interval
					target.tryAfter = after
					target.attemptCount += 1
					target.attempting = true;
					go cm.attemptConnection(address, target.commandLine)
				}
			}
			tick = nil
			maybeTick()
		}
	}
}

func (cm *ConnectionMaker) addToTargets(name PeerName, foundAt string) {
	target := cm.targets[foundAt]
	if target == nil {
		after, interval := tryAfter(InitialInterval)
		target = &Target{
			tryInterval: interval,
			tryAfter:    after}
	}
	foundAtHost, foundAtPortStr, err := net.SplitHostPort(foundAt)
	if err == nil {
		// ensure port-less version is there
		cm.targets[foundAtHost] = target
		if foundAtPort, err := strconv.Atoi(foundAtPortStr); err == nil && foundAtPort != Port {
		    cm.targets[foundAt] = target
		}
	} else {
		// can't split it, assume it must not have port on it
	    cm.targets[foundAt] = target
	}
}

func (cm *ConnectionMaker) status() string {
	var buf bytes.Buffer
	for address, target := range cm.targets {
		if (target.conn != nil) {
			buf.WriteString(fmt.Sprintf("%s connected to: %v\n", address, target.conn))
		} else if target.attempting {
			buf.WriteString(fmt.Sprintf("%s (%v attempts, trying again now)\n", address, target.attemptCount))
		} else {
			buf.WriteString(fmt.Sprintf("%s (%v attempts, next at %v)\n", address, target.attemptCount, target.tryAfter))
		}
	}
	return buf.String()
}

func (cm *ConnectionMaker) attemptConnection(address string, acceptNewPeer bool) {
	log.Println("Attempting connection to ", address)
	var conncode = CMConnSucceeded
	if err := cm.router.Ourself.CreateConnection(address, UnknownPeerName); err != nil {
		log.Println(err)
		conncode = CMConnFailed
	}
	// Tell the query loop we've finished this attempt
	cm.queryChan <- &ConnectionMakerInteraction{
		Interaction: Interaction{code: conncode},
		name:        UnknownPeerName,
		foundAt:     address}
}

func tryAfter(interval time.Duration) (time.Time, time.Duration) {
	interval += time.Duration(rand.Int63n(int64(interval)))
	if interval > MaxInterval {
		interval = MaxInterval
	}
	return time.Now().Add(interval), interval
}
