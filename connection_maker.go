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
		failedConnections: make(map[PeerName]*FailedConnection),
		attempting:        make(map[ConnectionMakerPair]bool)}
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
		if tick == nil && len(cm.failedConnections) > 0 {
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
					cm.addToFailedConnection(query.name, query.foundAt)
					maybeTick()
				}
			case query.code == CMStatus:
				query.resultChan <- cm.status()
			case query.code == CMConnSucceeded:
				delete(cm.attempting, ConnectionMakerPair{query.foundAt, query.name})
				delete(cm.failedConnections[query.name].foundAt, query.foundAt)
			case query.code == CMConnFailed:
				delete(cm.attempting, ConnectionMakerPair{query.foundAt, query.name})
			default:
				log.Fatal("Unexpected connection maker query:", query)
			}
		case now := <-tick:
		ConnectionLoop:
			for name, failedConn := range cm.failedConnections {
				if now.After(failedConn.tryAfter) {
					if _, found := cm.router.Ourself.ConnectionTo(name); found {
						delete(cm.failedConnections, name)
						continue
					} else if len(failedConn.foundAt) == 0 {
						delete(cm.failedConnections, name)
					} else if failedConn.attemptCount == MaxAttemptCount {
						delete(cm.failedConnections, name)
						continue
					}
					// wait until all current attempts to contact this peer have failed before starting a new batch
					for target := range failedConn.foundAt {
						if cm.attempting[ConnectionMakerPair{target, name}] {
							continue ConnectionLoop
						}
					}
					after, interval := tryAfter(failedConn.tryInterval)
					failedConn.tryInterval = interval
					failedConn.tryAfter = after
					failedConn.attemptCount += 1
					for target := range failedConn.foundAt {
						cm.attempting[ConnectionMakerPair{target, name}] = true
						go cm.attemptConnection(target, name)
					}
				}
			}
			tick = nil
			maybeTick()
		}
	}
}

func (cm *ConnectionMaker) addToFailedConnection(name PeerName, foundAt string) {
	failed := cm.failedConnections[name]
	if failed == nil {
		after, interval := tryAfter(InitialInterval)
		failed = &FailedConnection{
			foundAt:     make(map[string]bool),
			tryInterval: interval,
			tryAfter:    after}
	}
	foundAtHost, foundAtPortStr, err := net.SplitHostPort(foundAt)
	if err == nil {
		// ensure port-less version is there
		failed.foundAt[foundAtHost] = true
		if foundAtPort, err := strconv.Atoi(foundAtPortStr); err == nil && foundAtPort != Port {
			failed.foundAt[foundAt] = true
		}
	} else {
		// can't split it, assume it must not have port on it
		failed.foundAt[foundAt] = true
	}
	cm.failedConnections[name] = failed
}

func (cm *ConnectionMaker) status() string {
	var buf bytes.Buffer
	for name, failedConn := range cm.failedConnections {
		tryingNow := false
		foundAt := make([]string, 0, len(failedConn.foundAt))
		for target := range failedConn.foundAt {
			foundAt = append(foundAt, target)
			tryingNow = tryingNow || cm.attempting[ConnectionMakerPair{target, name}]
		}
		if tryingNow {
			buf.WriteString(fmt.Sprintf("%s (%v attempts, trying again now): %v\n", name, failedConn.attemptCount, foundAt))
		} else {
			buf.WriteString(fmt.Sprintf("%s (%v attempts, next at %v): %v\n", name, failedConn.attemptCount, failedConn.tryAfter, foundAt))
		}
	}
	return buf.String()
}

func (cm *ConnectionMaker) attemptConnection(foundAt string, targetName PeerName) {
	var conncode = CMConnSucceeded
	if err := cm.router.Ourself.CreateConnection(foundAt, targetName); err != nil {
		log.Println(err)
		conncode = CMConnFailed
	}
	// Tell the query loop we've finished this attempt
	cm.queryChan <- &ConnectionMakerInteraction{
		Interaction: Interaction{code: conncode},
		name:        targetName,
		foundAt:     foundAt}
}

func tryAfter(interval time.Duration) (time.Time, time.Duration) {
	interval += time.Duration(rand.Int63n(int64(interval)))
	if interval > MaxInterval {
		interval = MaxInterval
	}
	return time.Now().Add(interval), interval
}
