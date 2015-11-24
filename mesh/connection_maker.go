package mesh

import (
	"fmt"
	"math/rand"
	"net"
	"time"
)

const (
	InitialInterval = 2 * time.Second
	MaxInterval     = 6 * time.Minute
	ResetAfter      = 1 * time.Minute
)

type peerAddrs map[string]*net.TCPAddr

type ConnectionMaker struct {
	ourself     *LocalPeer
	peers       *Peers
	port        int
	discovery   bool
	targets     map[string]*Target
	connections map[Connection]struct{}
	directPeers peerAddrs
	actionChan  chan<- ConnectionMakerAction
}

type TargetState int

const (
	TargetWaiting    TargetState = iota // we are waiting to connect there
	TargetAttempting                    // we are attempting to connect there
	TargetConnected                     // we are connected to there
)

// Information about an address where we may find a peer
type Target struct {
	state       TargetState
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
		connections: make(map[Connection]struct{}),
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
			if target, found := cm.targets[cm.completeAddr(*addr)]; found {
				target.nextTryNow()
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

func (cm *ConnectionMaker) ConnectionAborted(address string, err error) {
	cm.actionChan <- func() bool {
		target := cm.targets[address]
		target.state = TargetWaiting
		target.lastError = err
		target.nextTryLater()
		return true
	}
}

func (cm *ConnectionMaker) ConnectionCreated(conn Connection) {
	cm.actionChan <- func() bool {
		cm.connections[conn] = void
		if conn.Outbound() {
			target := cm.targets[conn.RemoteTCPAddr()]
			target.state = TargetConnected
		}
		return false
	}
}

func (cm *ConnectionMaker) ConnectionTerminated(conn Connection, err error) {
	cm.actionChan <- func() bool {
		delete(cm.connections, conn)
		if conn.Outbound() {
			target := cm.targets[conn.RemoteTCPAddr()]
			target.state = TargetWaiting
			target.lastError = err
			switch {
			case err == ErrConnectToSelf:
				target.nextTryNever()
			case time.Now().After(target.tryAfter.Add(ResetAfter)):
				target.nextTryNow()
			default:
				target.nextTryLater()
			}
		}
		return true
	}
}

func (cm *ConnectionMaker) Refresh() {
	cm.actionChan <- func() bool { return true }
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

func (cm *ConnectionMaker) completeAddr(addr net.TCPAddr) string {
	if addr.Port == 0 {
		addr.Port = cm.port
	}
	return addr.String()
}

func (cm *ConnectionMaker) checkStateAndAttemptConnections() time.Duration {
	var (
		validTarget  = make(map[string]struct{})
		directTarget = make(map[string]struct{})
	)
	ourConnectedPeers, ourConnectedTargets, ourInboundIPs := cm.ourConnections()

	addTarget := func(address string) {
		if _, connected := ourConnectedTargets[address]; connected {
			return
		}
		validTarget[address] = void
		if _, found := cm.targets[address]; found {
			return
		}
		target := &Target{state: TargetWaiting}
		target.nextTryNow()
		cm.targets[address] = target
	}

	// Add direct targets that are not connected
	for _, addr := range cm.directPeers {
		attempt := true
		if addr.Port == 0 {
			// If a peer was specified w/o a port, then we do not
			// attempt to connect to it if we have any inbound
			// connections from that IP.
			if _, connected := ourInboundIPs[addr.IP.String()]; connected {
				attempt = false
			}
		}
		address := cm.completeAddr(*addr)
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
	for conn := range cm.connections {
		address := conn.RemoteTCPAddr()
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
				// that some peer has. Let's try to connect on the
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
		if target.state != TargetWaiting {
			continue
		}
		if _, valid := validTarget[address]; !valid {
			delete(cm.targets, address)
			continue
		}
		if target.tryAfter.IsZero() {
			continue
		}
		switch duration := target.tryAfter.Sub(now); {
		case duration <= 0:
			target.state = TargetAttempting
			_, isCmdLineTarget := directTarget[address]
			go cm.attemptConnection(address, isCmdLineTarget)
		case duration < after:
			after = duration
		}
	}
	return after
}

func (cm *ConnectionMaker) attemptConnection(address string, acceptNewPeer bool) {
	log.Printf("->[%s] attempting connection", address)
	if err := cm.ourself.CreateConnection(address, acceptNewPeer); err != nil {
		log.Errorf("->[%s] error during connection attempt: %v", address, err)
		cm.ConnectionAborted(address, err)
	}
}

func (t *Target) nextTryNever() {
	t.tryAfter = time.Time{}
	t.tryInterval = MaxInterval
}

func (t *Target) nextTryNow() {
	t.tryAfter = time.Now()
	t.tryInterval = InitialInterval
}

// The delay at the nth retry is a random value in the range
// [i-i/2,i+i/2], where i = InitialInterval * 1.5^(n-1).
func (t *Target) nextTryLater() {
	t.tryAfter = time.Now().Add(t.tryInterval/2 + time.Duration(rand.Int63n(int64(t.tryInterval))))
	t.tryInterval = t.tryInterval * 3 / 2
	if t.tryInterval > MaxInterval {
		t.tryInterval = MaxInterval
	}
}
