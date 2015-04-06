package router

import (
	"encoding/json"
	"fmt"
	"time"
)

func (router *Router) GenerateStatusJSON(version, encryption string) ([]byte, error) {
	return json.Marshal(struct {
		Version    string
		Encryption string
		Name       string
		NickName   string
		Interface  string
		Macs       *MacCache
		Peers      *Peers
		Routes     *Routes
	}{version, encryption, router.Ourself.Name.String(), router.Ourself.NickName, fmt.Sprintf("%v", router.Iface), router.Macs, router.Peers, router.Routes})
	// leaving out ConectionMaker due to async complexities
}

func (cache *MacCache) MarshalJSON() ([]byte, error) {
	type cacheEntry struct {
		Mac      string
		Name     string
		NickName string
		LastSeen time.Time
	}
	var entries []*cacheEntry
	for key, entry := range cache.table {
		entries = append(entries, &cacheEntry{intmac(key).String(), entry.peer.Name.String(), entry.peer.NickName, entry.lastSeen})
	}
	return json.Marshal(entries)
}

func (peers *Peers) MarshalJSON() ([]byte, error) {
	var ps []*Peer
	peers.ForEach(func(peer *Peer) { ps = append(ps, peer) })
	return json.Marshal(ps)
}

func (routes *Routes) MarshalJSON() ([]byte, error) {
	routes.RLock()
	defer routes.RUnlock()
	type uni struct {
		Dest, Via PeerName
	}
	type broad struct {
		Source PeerName
		Via    []PeerName
	}
	var r struct {
		Unicast   []*uni
		Broadcast []*broad
	}
	for name, hop := range routes.unicast {
		r.Unicast = append(r.Unicast, &uni{name, hop})
	}
	for name, hops := range routes.broadcast {
		r.Broadcast = append(r.Broadcast, &broad{name, hops})
	}
	return json.Marshal(r)
}

func (peer *Peer) MarshalJSON() ([]byte, error) {
	conns := peer.Connections()
	connections := make([]Connection, 0, len(conns))
	for conn := range conns {
		connections = append(connections, conn)
	}
	return json.Marshal(struct {
		Name        string
		NickName    string
		UID         uint64
		Version     uint64
		Connections []Connection
	}{peer.Name.String(), peer.NickName, peer.UID, peer.version, connections})
}

func (conn *RemoteConnection) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Name     string
		NickName string
		TCPAddr  string
	}{conn.Remote().Name.String(), conn.Remote().NickName, conn.RemoteTCPAddr()})
}

func (name PeerName) MarshalJSON() ([]byte, error) {
	return json.Marshal(name.String())
}
