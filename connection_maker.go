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
	CMInitiate      = iota
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
		Interaction: Interaction{code: CMInitiate},
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
			case query.code == CMInitiate:
				cm.addToTargets(true, query.address)
				tickNow()
			case query.code == CMStatus:
				query.resultChan <- cm.status()
			case query.code == CMConnFailed:
				target := cm.targets[query.address]
				if target != nil {
					target.state = CSUnconnected
					target.tryAfter, target.tryInterval = tryAfter(target.tryInterval)
					maybeTick()
				} else {
					log.Fatal("CMConnFailed unknown address", query.address)
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
	count_unconnected_commandline := 0
	// Any targets that are now connected, we don't need to attempt any more
	for address, target := range cm.targets {
		if _, found := ourself.ConnectionOn(address); found {
			if target.isCmdLine {
				// We need to keep hold of targets supplied on the
				// command-line, because that info isn't
				// tracked anywhere else
				target.state = CSEstablished
				target.tryInterval = InitialInterval
			} else {
				delete(cm.targets, address)
			}
		} else if target.isCmdLine {
			if target.state != CSAttempting {
				target.state = CSUnconnected
			}
			count_unconnected_commandline += 1
		}
	}

	// Look for peers that we don't have a connection to.
	count_unconnected_peers := 0
	cm.router.Peers.ForEach(func(name PeerName, peer *Peer) {
		for peer2, conn := range peer.connections {
			if _, found := ourself.ConnectionTo(peer2); !found &&
				peer2 != ourself.Name {
				log.Println("Unconnected peer:", peer2)
				count_unconnected_peers += 1
				// peer2 is a peer that someone else knows about, but we don't have a connection to.
				address := conn.RemoteTCPAddr()
				if host, port, err := ExtractHostPort(address); err == nil {
					if port != Port {
						cm.addToTargets(false, address)
					}
					cm.addToTargets(false, host)
				}
			}
		}
	})

	if count_unconnected_commandline == 0 && count_unconnected_peers == 0 {
		// We are already connected to everyone we could possibly be connected to; nothing further to do.
		return
	}

	// Now connect to any targets in scope
	for address, target := range cm.targets {
		if target.state == CSUnconnected && now.After(target.tryAfter) {
			target.attemptCount += 1
			target.state = CSAttempting
			go cm.attemptConnection(address, target.isCmdLine)
		}
	}
}

func (cm *ConnectionMaker) addToTargets(isCmdLine bool, address string) {
	target := cm.targets[address]
	if target == nil {
		target = &Target{
			state:     CSUnconnected,
			isCmdLine: isCmdLine,
		}
		target.tryAfter, target.tryInterval = tryImmediately()
		cm.targets[address] = target
	}
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
