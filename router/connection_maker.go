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

type ConnectionMaker struct {
	ourself           *LocalPeer
	peers             *Peers
	normalisePeerAddr func(string) string
	targets           map[string]*Target
	cmdLinePeers      map[string]string
	actionChan        chan<- ConnectionMakerAction
}

// Information about an address where we may find a peer
type Target struct {
	attempting  bool          // are we currently attempting to connect there?
	lastError   error         // reason for disconnection last time
	tryAfter    time.Time     // next time to try this address
	tryInterval time.Duration // backoff time on next failure
}

type ConnectionMakerAction func() bool

func NewConnectionMaker(ourself *LocalPeer, peers *Peers, normalisePeerAddr func(string) string) *ConnectionMaker {
	return &ConnectionMaker{
		ourself:           ourself,
		peers:             peers,
		normalisePeerAddr: normalisePeerAddr,
		cmdLinePeers:      make(map[string]string),
		targets:           make(map[string]*Target)}
}

func (cm *ConnectionMaker) Start() {
	actionChan := make(chan ConnectionMakerAction, ChannelSize)
	cm.actionChan = actionChan
	go cm.queryLoop(actionChan)
}

func (cm *ConnectionMaker) InitiateConnection(peer string) error {
	addr, err := net.ResolveTCPAddr("tcp4", cm.normalisePeerAddr(peer))
	if err != nil {
		return err
	}
	address := addr.String()

	cm.actionChan <- func() bool {
		cm.cmdLinePeers[peer] = address
		// curtail any existing reconnect interval
		if target, found := cm.targets[address]; found {
			target.tryAfter, target.tryInterval = tryImmediately()
		}
		return true
	}
	return nil
}

func (cm *ConnectionMaker) ForgetConnection(peer string) {
	cm.actionChan <- func() bool {
		delete(cm.cmdLinePeers, peer)
		return false
	}
}

func (cm *ConnectionMaker) ConnectionTerminated(address string, err error) {
	cm.actionChan <- func() bool {
		if target, found := cm.targets[address]; found {
			target.attempting = false
			target.lastError = err
			target.tryAfter, target.tryInterval = tryAfter(target.tryInterval)
		}
		return true
	}
}

func (cm *ConnectionMaker) Refresh() {
	cm.actionChan <- func() bool { return true }
}

func (cm *ConnectionMaker) String() string {
	// We need to Refresh first in order to clear out any 'attempting'
	// connections from cm.targets that have been established since
	// the last run of cm.checkStateAndAttemptConnections. These
	// entries are harmless but do represent stale state that we do
	// not want to report.
	cm.Refresh()
	resultChan := make(chan string, 0)
	cm.actionChan <- func() bool {
		var buf bytes.Buffer
		for address, target := range cm.targets {
			fmt.Fprint(&buf, address)
			if target.lastError != nil {
				fmt.Fprintf(&buf, " (%s)", target.lastError)
			}
			if target.attempting {
				fmt.Fprintf(&buf, " (trying since %v)\n", target.tryAfter)
			} else {
				fmt.Fprintf(&buf, " (next try at %v)\n", target.tryAfter)
			}
		}
		resultChan <- buf.String()
		return false
	}
	return <-resultChan
}

func (cm *ConnectionMaker) queryLoop(actionChan <-chan ConnectionMakerAction) {
	timer := time.NewTimer(MaxDuration)
	run := func() { timer.Reset(cm.checkStateAndAttemptConnections()) }
	for {
		select {
		case action := <-actionChan:
			if action() {
				run()
			}
		case <-timer.C:
			run()
		}
	}
}

func (cm *ConnectionMaker) checkStateAndAttemptConnections() time.Duration {
	validTarget := make(map[string]struct{})
	cmdLineTarget := make(map[string]struct{})

	// copy the set of things we are connected to, so we can access them without locking
	ourConnectedPeers := make(PeerNameSet)
	ourConnectedTargets := make(map[string]struct{})
	for conn := range cm.ourself.Connections() {
		address := conn.RemoteTCPAddr()
		ourConnectedPeers[conn.Remote().Name] = void
		ourConnectedTargets[address] = void
		delete(cm.targets, address)
	}

	addTarget := func(address string) {
		if _, connected := ourConnectedTargets[address]; connected {
			return
		}
		validTarget[address] = void
		if _, found := cm.targets[address]; found {
			return
		}
		target := &Target{}
		target.tryAfter, target.tryInterval = tryImmediately()
		cm.targets[address] = target
	}

	// Add command-line targets that are not connected
	for _, address := range cm.cmdLinePeers {
		addTarget(address)
		cmdLineTarget[address] = void
	}

	// Add targets for peers that someone else is connected to, but we
	// aren't
	cm.peers.ForEach(func(peer *Peer) {
		for conn := range peer.Connections() {
			otherPeer := conn.Remote().Name
			if otherPeer == cm.ourself.Name {
				continue
			}
			if _, connected := ourConnectedPeers[otherPeer]; connected {
				continue
			}
			address := conn.RemoteTCPAddr()
			// try both portnumber of connection and standard port.  Don't use remote side of inbound connection.
			if conn.Outbound() {
				addTarget(address)
			}
			if host, _, err := net.SplitHostPort(address); err == nil {
				addTarget(cm.normalisePeerAddr(host))
			}
		}
	})

	return cm.connectToTargets(validTarget, cmdLineTarget)
}

func (cm *ConnectionMaker) addTarget(address string) {
}

func (cm *ConnectionMaker) connectToTargets(validTarget map[string]struct{}, cmdLineTarget map[string]struct{}) time.Duration {
	now := time.Now() // make sure we catch items just added
	after := MaxDuration
	for address, target := range cm.targets {
		if target.attempting {
			continue
		}
		if _, valid := validTarget[address]; !valid {
			delete(cm.targets, address)
			continue
		}
		switch duration := target.tryAfter.Sub(now); {
		case duration <= 0:
			target.attempting = true
			_, isCmdLineTarget := cmdLineTarget[address]
			go cm.attemptConnection(address, isCmdLineTarget)
		case duration < after:
			after = duration
		}
	}
	return after
}

func (cm *ConnectionMaker) attemptConnection(address string, acceptNewPeer bool) {
	log.Printf("->[%s] attempting connection\n", address)
	if err := cm.ourself.CreateConnection(address, acceptNewPeer); err != nil {
		log.Printf("->[%s] error during connection attempt: %v\n", address, err)
		cm.ConnectionTerminated(address, err)
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
