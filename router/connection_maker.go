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
	InitialInterval = 2 * time.Second
	MaxInterval     = 6 * time.Minute
)

type peerAddrs map[string]*net.TCPAddr

type ConnectionMaker struct {
	ourself     *LocalPeer
	peers       *Peers
	port        int
	discovery   bool
	targets     map[string]*Target
	directPeers peerAddrs
	actionChan  chan<- ConnectionMakerAction
}

// Information about an address where we may find a peer
type Target struct {
	attempting  bool          // are we currently attempting to connect there?
	lastError   error         // reason for disconnection last time
	tryAfter    time.Time     // next time to try this address
	tryInterval time.Duration // retry delay on next failure
}

type ConnectionMakerAction func() bool

func NewConnectionMaker(ourself *LocalPeer, peers *Peers, port int, discovery bool) *ConnectionMaker {
	actionChan := make(chan ConnectionMakerAction, ChannelSize)
	cm := &ConnectionMaker{
		ourself:     ourself,
		peers:       peers,
		port:        port,
		discovery:   discovery,
		directPeers: peerAddrs{},
		targets:     make(map[string]*Target),
		actionChan:  actionChan}
	go cm.queryLoop(actionChan)
	return cm
}

func (cm *ConnectionMaker) InitiateConnections(peers []string, replace bool) []error {
	errors := []error{}
	addrs := peerAddrs{}
	for _, peer := range peers {
		host, port, err := net.SplitHostPort(peer)
		if err != nil {
			host = peer
			port = "0" // we use that as an indication that "no port was supplied"
		}
		if addr, err := net.ResolveTCPAddr("tcp4", fmt.Sprintf("%s:%s", host, port)); err != nil {
			errors = append(errors, err)
		} else {
			addrs[peer] = addr
		}
	}
	cm.actionChan <- func() bool {
		if replace {
			cm.directPeers = peerAddrs{}
		}
		for peer, addr := range addrs {
			cm.directPeers[peer] = addr
			// curtail any existing reconnect interval
			if target, found := cm.targets[addr.String()]; found {
				target.tryNow()
			}
		}
		return true
	}
	return errors
}

func (cm *ConnectionMaker) ForgetConnections(peers []string) {
	cm.actionChan <- func() bool {
		for _, peer := range peers {
			delete(cm.directPeers, peer)
		}
		return false
	}
}

func (cm *ConnectionMaker) ConnectionTerminated(address string, err error) {
	cm.actionChan <- func() bool {
		if target, found := cm.targets[address]; found {
			target.attempting = false
			target.lastError = err
			target.retry()
		}
		return true
	}
}

func (cm *ConnectionMaker) Refresh() {
	cm.actionChan <- func() bool { return true }
}

func (cm *ConnectionMaker) Status() ConnectionMakerStatus {
	// We need to Refresh first in order to clear out any 'attempting'
	// connections from cm.targets that have been established since
	// the last run of cm.checkStateAndAttemptConnections. These
	// entries are harmless but do represent stale state that we do
	// not want to report.
	cm.Refresh()
	resultChan := make(chan ConnectionMakerStatus, 0)
	cm.actionChan <- func() bool {
		status := ConnectionMakerStatus{
			DirectPeers: []string{},
			Reconnects:  make(map[string]Target),
		}
		for peer := range cm.directPeers {
			status.DirectPeers = append(status.DirectPeers, peer)
		}

		for address, target := range cm.targets {
			status.Reconnects[address] = *target
		}
		resultChan <- status
		return false
	}
	return <-resultChan
}

type ConnectionMakerStatus struct {
	DirectPeers []string
	Reconnects  map[string]Target
}

func (status ConnectionMakerStatus) String() string {
	var buf bytes.Buffer
	fmt.Fprint(&buf, "Direct Peers:")
	for _, peer := range status.DirectPeers {
		fmt.Fprintf(&buf, " %s", peer)
	}
	fmt.Fprintln(&buf, "\nReconnects:")
	for address, target := range status.Reconnects {
		fmt.Fprintf(&buf, "->[%s]", address)
		if target.lastError != nil {
			fmt.Fprintf(&buf, " (%s)", target.lastError)
		}
		if target.attempting {
			fmt.Fprintf(&buf, " trying since %v\n", target.tryAfter)
		} else {
			fmt.Fprintf(&buf, " next try at %v\n", target.tryAfter)
		}
	}
	return buf.String()
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
	var (
		validTarget  = make(map[string]struct{})
		directTarget = make(map[string]struct{})
	)
	// Copy the set of things we are connected to, so we can access
	// them without locking.  Also clear out any entries in cm.targets
	// for existing connections.
	ourConnectedPeers, ourConnectedTargets, ourInboundIPs := cm.ourConnections()

	addTarget := func(address string) {
		if _, connected := ourConnectedTargets[address]; connected {
			return
		}
		validTarget[address] = void
		if _, found := cm.targets[address]; found {
			return
		}
		target := &Target{}
		target.tryNow()
		cm.targets[address] = target
	}

	// Add direct targets that are not connected
	for _, addr := range cm.directPeers {
		completeAddr := *addr
		attempt := true
		if completeAddr.Port == 0 {
			completeAddr.Port = cm.port
			// If a peer was specified w/o a port, then we do not
			// attempt to connect to it if we have any inbound
			// connections from that IP.
			if _, connected := ourInboundIPs[completeAddr.IP.String()]; connected {
				attempt = false
			}
		}
		address := completeAddr.String()
		directTarget[address] = void
		if attempt {
			addTarget(address)
		}
	}

	// Add targets for peers that someone else is connected to, but we
	// aren't
	if cm.discovery {
		cm.addPeerTargets(ourConnectedPeers, addTarget)
	}

	return cm.connectToTargets(validTarget, directTarget)
}

func (cm *ConnectionMaker) ourConnections() (PeerNameSet, map[string]struct{}, map[string]struct{}) {
	var (
		ourConnectedPeers   = make(PeerNameSet)
		ourConnectedTargets = make(map[string]struct{})
		ourInboundIPs       = make(map[string]struct{})
	)
	for conn := range cm.ourself.Connections() {
		address := conn.RemoteTCPAddr()
		delete(cm.targets, address)
		ourConnectedPeers[conn.Remote().Name] = void
		ourConnectedTargets[address] = void
		if conn.Outbound() {
			continue
		}
		if ip, _, err := net.SplitHostPort(address); err == nil { // should always succeed
			ourInboundIPs[ip] = void
		}
	}
	return ourConnectedPeers, ourConnectedTargets, ourInboundIPs
}

func (cm *ConnectionMaker) addPeerTargets(ourConnectedPeers PeerNameSet, addTarget func(string)) {
	cm.peers.ForEach(func(peer *Peer) {
		if peer == cm.ourself.Peer {
			return
		}
		// Modifying peer.connections requires a write lock on Peers,
		// and since we are holding a read lock (due to the ForEach),
		// access without locking the peer is safe.
		for otherPeer, conn := range peer.connections {
			if otherPeer == cm.ourself.Name {
				continue
			}
			if _, connected := ourConnectedPeers[otherPeer]; connected {
				continue
			}
			address := conn.RemoteTCPAddr()
			if conn.Outbound() {
				addTarget(address)
			} else if ip, _, err := net.SplitHostPort(address); err == nil {
				// There is no point connecting to the (likely
				// ephemeral) remote port of an inbound connection
				// that some peer has. Let's try to connect to on the
				// weave port instead.
				addTarget(fmt.Sprintf("%s:%d", ip, cm.port))
			}
		}
	})
}

func (cm *ConnectionMaker) connectToTargets(validTarget map[string]struct{}, directTarget map[string]struct{}) time.Duration {
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
			_, isCmdLineTarget := directTarget[address]
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

func (t *Target) tryNow() {
	t.tryAfter = time.Now()
	t.tryInterval = InitialInterval
}

// The delay at the nth retry is a random value in the range
// [i-i/2,i+i/2], where i = InitialInterval * 1.5^(n-1).
func (t *Target) retry() {
	t.tryAfter = time.Now().Add(t.tryInterval/2 + time.Duration(rand.Int63n(int64(t.tryInterval))))
	t.tryInterval = t.tryInterval * 3 / 2
	if t.tryInterval > MaxInterval {
		t.tryInterval = MaxInterval
	}
}
