package router

import (
	"fmt"
	"time"
)

type Status struct {
	Protocol           string
	ProtocolMinVersion int
	ProtocolMaxVersion int
	Encryption         bool
	PeerDiscovery      bool
	Name               string
	NickName           string
	Port               int
	Interface          string
	CaptureStats       map[string]int
	MACs               []MACStatus
	Peers              []PeerStatus
	UnicastRoutes      []UnicastRouteStatus
	BroadcastRoutes    []BroadcastRouteStatus
	Connections        []LocalConnectionStatus
	Targets            []string
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

type LocalConnectionStatus struct {
	Address  string
	Outbound bool
	State    string
	Info     string
}

func NewStatus(router *Router) *Status {
	return &Status{
		Protocol,
		ProtocolMinVersion,
		ProtocolMaxVersion,
		router.UsingPassword(),
		router.PeerDiscovery,
		router.Ourself.Name.String(),
		router.Ourself.NickName,
		router.Port,
		router.Bridge.String(),
		router.Bridge.Stats(),
		NewMACStatusSlice(router.Macs),
		NewPeerStatusSlice(router.Peers),
		NewUnicastRouteStatusSlice(router.Routes),
		NewBroadcastRouteStatusSlice(router.Routes),
		NewLocalConnectionStatusSlice(router.ConnectionMaker),
		NewTargetSlice(router.ConnectionMaker)}
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
			peer.Version,
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

func NewLocalConnectionStatusSlice(cm *ConnectionMaker) []LocalConnectionStatus {
	resultChan := make(chan []LocalConnectionStatus, 0)
	cm.actionChan <- func() bool {
		var slice []LocalConnectionStatus
		for conn := range cm.connections {
			state := "pending"
			if conn.Established() {
				state = "established"
			}
			lc, _ := conn.(*LocalConnection)
			info := fmt.Sprintf("%-6v %v", lc.forwarder.OverlayType(), conn.Remote())
			slice = append(slice, LocalConnectionStatus{conn.RemoteTCPAddr(), conn.Outbound(), state, info})
		}
		for address, target := range cm.targets {
			add := func(state, info string) {
				slice = append(slice, LocalConnectionStatus{address, true, state, info})
			}
			switch target.state {
			case TargetWaiting:
				until := "never"
				if !target.tryAfter.IsZero() {
					until = target.tryAfter.String()
				}
				if target.lastError == nil { // shouldn't happen
					add("waiting", "until: "+until)
				} else {
					add("failed", target.lastError.Error()+", retry: "+until)
				}
			case TargetAttempting:
				if target.lastError == nil {
					add("connecting", "")
				} else {
					add("retrying", target.lastError.Error())
				}
			case TargetConnected:
			}
		}
		resultChan <- slice
		return false
	}
	return <-resultChan
}

func NewTargetSlice(cm *ConnectionMaker) []string {
	resultChan := make(chan []string, 0)
	cm.actionChan <- func() bool {
		var slice []string
		for peer := range cm.directPeers {
			slice = append(slice, peer)
		}
		resultChan <- slice
		return false
	}
	return <-resultChan
}
