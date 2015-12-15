package mesh

import (
	"fmt"
	"math"
	"net"
	"sync"
	"time"

	"github.com/weaveworks/weave/common"
)

const (
	Port           = 6783
	ChannelSize    = 16
	TCPHeartbeat   = 30 * time.Second
	GossipInterval = 30 * time.Second
	MaxDuration    = time.Duration(math.MaxInt64)

	acceptMaxTokens  = 100                    // [1]
	acceptTokenDelay = 100 * time.Millisecond // [2]
)

// [1] capacity of token bucket for rate limiting accepts

// [2] control rate at which new tokens are added to the bucket

var (
	log        = common.Log
	checkFatal = common.CheckFatal
	checkWarn  = common.CheckWarn
)

type Config struct {
	Port               int
	ProtocolMinVersion byte
	Password           []byte
	ConnLimit          int
	PeerDiscovery      bool
	TrustedSubnets     []*net.IPNet
}

type Router struct {
	Config
	Overlay         Overlay
	Ourself         *LocalPeer
	Peers           *Peers
	Routes          *Routes
	ConnectionMaker *ConnectionMaker
	gossipLock      sync.RWMutex
	gossipChannels  GossipChannels
	TopologyGossip  Gossip
	acceptLimiter   *TokenBucket
}

func NewRouter(config Config, name PeerName, nickName string, overlay Overlay) *Router {
	router := &Router{Config: config, gossipChannels: make(GossipChannels)}

	if overlay == nil {
		overlay = NullOverlay{}
	}

	router.Overlay = overlay
	router.Ourself = NewLocalPeer(name, nickName, router)
	router.Peers = NewPeers(router.Ourself)
	router.Peers.OnGC(func(peer *Peer) {
		log.Println("Removed unreachable peer", peer)
	})
	router.Routes = NewRoutes(router.Ourself, router.Peers)
	router.ConnectionMaker = NewConnectionMaker(router.Ourself, router.Peers, router.Port, router.PeerDiscovery)
	router.TopologyGossip = router.NewGossip("topology", router)
	router.acceptLimiter = NewTokenBucket(acceptMaxTokens, acceptTokenDelay)

	return router
}

// Start listening for TCP connections. This is separate from
// NewRouter so that gossipers can register before we start forming
// connections.
func (router *Router) Start() {
	router.listenTCP(router.Port)
}

func (router *Router) Stop() error {
	// TODO: perform graceful shutdown...
	return nil
}

func (router *Router) UsingPassword() bool {
	return router.Password != nil
}

func (router *Router) listenTCP(localPort int) {
	localAddr, err := net.ResolveTCPAddr("tcp4", fmt.Sprint(":", localPort))
	checkFatal(err)
	ln, err := net.ListenTCP("tcp4", localAddr)
	checkFatal(err)
	go func() {
		defer ln.Close()
		for {
			tcpConn, err := ln.AcceptTCP()
			if err != nil {
				log.Errorln(err)
				continue
			}
			router.acceptTCP(tcpConn)
			router.acceptLimiter.Wait()
		}
	}()
}

func (router *Router) acceptTCP(tcpConn *net.TCPConn) {
	remoteAddrStr := tcpConn.RemoteAddr().String()
	log.Printf("->[%s] connection accepted", remoteAddrStr)
	connRemote := NewRemoteConnection(router.Ourself.Peer, nil, remoteAddrStr, false, false)
	StartLocalConnection(connRemote, tcpConn, router, true)
}

// Gossiper methods - the Router is the topology Gossiper

type TopologyGossipData struct {
	peers  *Peers
	update PeerNameSet
}

func (d *TopologyGossipData) Merge(other GossipData) GossipData {
	names := make(PeerNameSet)
	for name := range d.update {
		names[name] = void
	}
	for name := range other.(*TopologyGossipData).update {
		names[name] = void
	}
	return &TopologyGossipData{peers: d.peers, update: names}
}

func (d *TopologyGossipData) Encode() [][]byte {
	return [][]byte{d.peers.EncodePeers(d.update)}
}

func (router *Router) BroadcastTopologyUpdate(update []*Peer) {
	names := make(PeerNameSet)
	for _, p := range update {
		names[p.Name] = void
	}
	router.TopologyGossip.GossipBroadcast(
		&TopologyGossipData{peers: router.Peers, update: names})
}

func (router *Router) OnGossipUnicast(sender PeerName, msg []byte) error {
	return fmt.Errorf("unexpected topology gossip unicast: %v", msg)
}

func (router *Router) OnGossipBroadcast(_ PeerName, update []byte) (GossipData, error) {
	origUpdate, _, err := router.applyTopologyUpdate(update)
	if err != nil || len(origUpdate) == 0 {
		return nil, err
	}
	return &TopologyGossipData{peers: router.Peers, update: origUpdate}, nil
}

func (router *Router) Gossip() GossipData {
	return &TopologyGossipData{peers: router.Peers, update: router.Peers.Names()}
}

func (router *Router) OnGossip(update []byte) (GossipData, error) {
	_, newUpdate, err := router.applyTopologyUpdate(update)
	if err != nil || len(newUpdate) == 0 {
		return nil, err
	}
	return &TopologyGossipData{peers: router.Peers, update: newUpdate}, nil
}

func (router *Router) applyTopologyUpdate(update []byte) (PeerNameSet, PeerNameSet, error) {
	origUpdate, newUpdate, err := router.Peers.ApplyUpdate(update)
	if err != nil {
		return nil, nil, err
	}
	if len(newUpdate) > 0 {
		router.ConnectionMaker.Refresh()
		router.Routes.Recalculate()
	}
	return origUpdate, newUpdate, nil
}

func (router *Router) Trusts(remote *RemoteConnection) bool {
	if tcpAddr, err := net.ResolveTCPAddr("tcp4", remote.remoteTCPAddr); err == nil {
		for _, trustedSubnet := range router.TrustedSubnets {
			if trustedSubnet.Contains(tcpAddr.IP) {
				return true
			}
		}
	} else {
		// Should not happen as remoteTCPAddr was obtained from TCPConn
		log.Errorf("Unable to parse remote TCP addr: %s", err)
	}
	return false
}
