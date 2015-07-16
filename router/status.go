package router

import (
	"net"
	"time"
)

type Status struct {
	Encryption      bool
	PeerDiscovery   bool
	Name            string
	NickName        string
	Port            int
	Interface       *net.Interface
	MACs            []MACStatus
	Peers           []PeerStatus
	UnicastRoutes   []UnicastRouteStatus
	BroadcastRoutes []BroadcastRouteStatus
	ConnectionMaker ConnectionMakerStatus
}

type ConnectionMakerStatus struct {
	DirectPeers []string
	Reconnects  []TargetStatus
}

type TargetStatus struct {
	Address     string
	Attempting  bool      `json:"Attempting,omitempty"`
	TryAfter    time.Time `json:"TryAfter,omitempty"`
	TryInterval string    `json:"TryInterval,omitempty"`
	LastError   string    `json:"LastError,omitempty"`
}

type MACStatus struct {
	Mac      string
	Name     string
	NickName string
	LastSeen time.Time
}

type PeerStatus struct {
	Name        string
	NickName    string
	UID         PeerUID
	Version     uint64
	Connections []ConnectionStatus
}

type ConnectionStatus struct {
	Name        string
	NickName    string
	Address     string
	Outbound    bool
	Established bool
}

type UnicastRouteStatus struct {
	Dest, Via string
}

type BroadcastRouteStatus struct {
	Source string
	Via    []string
}

func NewStatus(router *Router) *Status {
	return &Status{
		router.UsingPassword(),
		router.PeerDiscovery,
		router.Ourself.Name.String(),
		router.Ourself.NickName,
		router.Port,
		router.Iface,
		NewMACStatusSlice(router.Macs),
		NewPeerStatusSlice(router.Peers),
		NewUnicastRouteStatusSlice(router.Routes),
		NewBroadcastRouteStatusSlice(router.Routes),
		NewConnectionMakerStatus(router.ConnectionMaker)}
}

func NewConnectionMakerStatus(cm *ConnectionMaker) ConnectionMakerStatus {
	// We need to Refresh first in order to clear out any 'attempting'
	// connections from cm.targets that have been established since
	// the last run of cm.checkStateAndAttemptConnections. These
	// entries are harmless but do represent stale state that we do
	// not want to report.
	cm.Refresh()
	resultChan := make(chan ConnectionMakerStatus, 0)
	cm.actionChan <- func() bool {
		status := ConnectionMakerStatus{
			[]string{},
			[]TargetStatus{},
		}
		for peer := range cm.directPeers {
			status.DirectPeers = append(status.DirectPeers, peer)
		}

		for address, target := range cm.targets {
			status.Reconnects = append(status.Reconnects, newTargetStatus(address, target))
		}
		resultChan <- status
		return false
	}
	return <-resultChan
}

func newTargetStatus(address string, target *Target) TargetStatus {
	var lastError string
	if target.lastError != nil {
		lastError = target.lastError.Error()
	}
	return TargetStatus{
		address,
		target.attempting,
		target.tryAfter,
		target.tryInterval.String(),
		lastError}
}

func NewMACStatusSlice(cache *MacCache) []MACStatus {
	cache.RLock()
	defer cache.RUnlock()

	var slice []MACStatus
	for key, entry := range cache.table {
		slice = append(slice, MACStatus{
			intmac(key).String(),
			entry.peer.Name.String(),
			entry.peer.NickName,
			entry.lastSeen})
	}

	return slice
}

func NewPeerStatusSlice(peers *Peers) []PeerStatus {
	var slice []PeerStatus

	peers.ForEach(func(peer *Peer) {
		var connections []ConnectionStatus
		if peer == peers.ourself.Peer {
			for conn := range peers.ourself.Connections() {
				connections = append(connections, newConnectionStatus(conn))
			}
		} else {
			// Modifying peer.connections requires a write lock on
			// Peers, and since we are holding a read lock (due to the
			// ForEach), access without locking the peer is safe.
			for _, conn := range peer.connections {
				connections = append(connections, newConnectionStatus(conn))
			}
		}
		slice = append(slice, PeerStatus{
			peer.Name.String(),
			peer.NickName,
			peer.UID,
			peer.version,
			connections})
	})

	return slice
}

func newConnectionStatus(c Connection) ConnectionStatus {
	return ConnectionStatus{
		c.Remote().Name.String(),
		c.Remote().NickName,
		c.RemoteTCPAddr(),
		c.Outbound(),
		c.Established()}
}

func NewUnicastRouteStatusSlice(routes *Routes) []UnicastRouteStatus {
	routes.RLock()
	defer routes.RUnlock()

	var slice []UnicastRouteStatus
	for dest, via := range routes.unicast {
		slice = append(slice, UnicastRouteStatus{dest.String(), via.String()})
	}
	return slice
}

func NewBroadcastRouteStatusSlice(routes *Routes) []BroadcastRouteStatus {
	routes.RLock()
	defer routes.RUnlock()

	var slice []BroadcastRouteStatus
	for source, via := range routes.broadcast {
		var hops []string
		for _, hop := range via {
			hops = append(hops, hop.String())
		}
		slice = append(slice, BroadcastRouteStatus{source.String(), hops})
	}
	return slice
}
