package nameserver

import (
	"fmt"
	"sync"
	"time"

	"github.com/miekg/dns"

	. "github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/mesh"
	"github.com/weaveworks/weave/net/address"
)

const (
	// Tombstones do not need to survive long periods of peer disconnection, as
	// we delete entries for disconnected peers.  Therefore they just need to hang
	// around to account for propagation delay through gossip.  We do need them
	// to hang around longer than the allowed clock skew (defined next).  30mins
	// tombstone timeout allows for 15mins max clock skew.
	tombstoneTimeout = time.Minute * 30

	// Maximum age of acceptable gossip messages (to account for clock skew)
	gossipWindow = int64(tombstoneTimeout/time.Second) / 2

	// Used by prog/weaver/main.go and proxy/create_container_interceptor.go
	DefaultDomain = "weave.local."
)

// Nameserver: gossip-based, in memory nameserver.
// - Holds a sorted list of (hostname, peer, container id, ip) tuples for the whole cluster.
// - This list is gossiped & merged around the cluser.
// - Lookup-by-hostname are O(nlogn), and return a (copy of a) slice of the entries
// - Update is O(n) for now
type Nameserver struct {
	sync.RWMutex
	ourName     mesh.PeerName
	domain      string
	gossip      mesh.Gossip
	entries     Entries
	isKnownPeer func(mesh.PeerName) bool
	quit        chan struct{}
}

func New(ourName mesh.PeerName, domain string, isKnownPeer func(mesh.PeerName) bool) *Nameserver {
	return &Nameserver{
		ourName:     ourName,
		domain:      dns.Fqdn(domain),
		isKnownPeer: isKnownPeer,
		quit:        make(chan struct{}),
	}
}

func (n *Nameserver) SetGossip(gossip mesh.Gossip) {
	n.gossip = gossip
}

func (n *Nameserver) Start() {
	go func() {
		ticker := time.Tick(tombstoneTimeout)
		for {
			select {
			case <-n.quit:
				return
			case <-ticker:
				n.deleteTombstones()
			}
		}
	}()
}

func (n *Nameserver) Stop() {
	n.quit <- struct{}{}
}

func (n *Nameserver) broadcastEntries(es ...Entry) error {
	if n.gossip == nil || len(es) == 0 {
		return nil
	}
	return n.gossip.GossipBroadcast((&GossipData{Timestamp: now(), Entries: es}).Encode())
}

func (n *Nameserver) AddEntry(hostname, containerid string, origin mesh.PeerName, addr address.Address) error {
	n.infof("adding entry %s -> %s", hostname, addr.String())
	n.Lock()
	entry := n.entries.add(hostname, containerid, origin, addr)
	n.Unlock()
	return n.broadcastEntries(entry)
}

func (n *Nameserver) Lookup(hostname string) []address.Address {
	n.RLock()
	defer n.RUnlock()

	entries := n.entries.lookup(hostname)
	result := []address.Address{}
	for _, e := range entries {
		if e.Tombstone > 0 {
			continue
		}
		result = append(result, e.Addr)
	}
	n.debugf("lookup %s -> %s", hostname, &result)
	return result
}

func (n *Nameserver) ReverseLookup(ip address.Address) (string, error) {
	n.RLock()
	defer n.RUnlock()

	match, err := n.entries.first(func(e *Entry) bool {
		return e.Tombstone == 0 && e.Addr == ip
	})
	if err != nil {
		return "", err
	}
	n.debugf("reverse lookup %s -> %s", ip, match.Hostname)
	return match.Hostname, nil
}

func (n *Nameserver) ContainerStarted(ident string) {}

func (n *Nameserver) ContainerDied(ident string) {
	n.Lock()
	entries := n.entries.tombstone(n.ourName, func(e *Entry) bool {
		if e.ContainerID == ident {
			n.infof("container %s died; tombstoning entry %s", ident, e.String())
			return true
		}
		return false
	})
	n.Unlock()
	if err := n.broadcastEntries(entries...); err != nil {
		n.errorf("failed to broadcast container %s death: %v", ident, err)
	}
}

func (n *Nameserver) PeerGone(peer mesh.PeerName) {
	n.infof("peer %s gone", peer.String())
	n.Lock()
	defer n.Unlock()
	n.entries.filter(func(e *Entry) bool {
		return e.Origin != peer
	})
}

func (n *Nameserver) Delete(hostname, containerid, ipStr string, ip address.Address) error {
	n.Lock()
	n.infof("tombstoning hostname=%s, container=%s, ip=%s", hostname, containerid, ipStr)
	entries := n.entries.tombstone(n.ourName, func(e *Entry) bool {
		if hostname != "*" && e.Hostname != hostname {
			return false
		}

		if containerid != "*" && e.ContainerID != containerid {
			return false
		}

		if ipStr != "*" && e.Addr != ip {
			return false
		}

		n.infof("tombstoning entry %v", e)
		return true
	})
	n.Unlock()
	return n.broadcastEntries(entries...)
}

func (n *Nameserver) deleteTombstones() {
	n.Lock()
	defer n.Unlock()
	now := time.Now().Unix()
	n.entries.filter(func(e *Entry) bool {
		return e.Tombstone == 0 || now-e.Tombstone <= int64(tombstoneTimeout/time.Second)
	})
}

func (n *Nameserver) Gossip() []byte {
	n.RLock()
	defer n.RUnlock()
	return (&GossipData{Timestamp: now(), Entries: n.entries}).Encode()
}

func (n *Nameserver) OnGossipUnicast(sender mesh.PeerName, msg []byte) error {
	return nil
}

// merge received data into state and return a representation of
// the received data, for further propagation
func (n *Nameserver) OnGossipBroadcast(_ mesh.PeerName, msg []byte) error {
	_, err := n.OnGossip(msg)
	return err
}

// merge received data into state and return "everything new I've
// just learnt", or nil if nothing in the received data was new
func (n *Nameserver) OnGossip(msg []byte) ([]byte, error) {
	var gossip GossipData
	if err := gossip.Decode(msg); err != nil {
		return nil, err
	}
	if delta := gossip.Timestamp - now(); delta > gossipWindow || delta < -gossipWindow {
		return nil, fmt.Errorf("clock skew of %d detected", delta)
	}

	n.Lock()
	defer n.Unlock()

	gossip.Entries.filter(func(e *Entry) bool {
		return n.isKnownPeer(e.Origin)
	})

	newEntries := n.entries.merge(gossip.Entries)
	if len(newEntries) == 0 {
		return nil, nil
	}
	return (&GossipData{Timestamp: now(), Entries: newEntries}).Encode(), nil
}

func (n *Nameserver) infof(fmt string, args ...interface{}) {
	Log.Infof("[nameserver %s] "+fmt, append([]interface{}{n.ourName}, args...)...)
}
func (n *Nameserver) debugf(fmt string, args ...interface{}) {
	Log.Debugf("[nameserver %s] "+fmt, append([]interface{}{n.ourName}, args...)...)
}
func (n *Nameserver) errorf(fmt string, args ...interface{}) {
	Log.Errorf("[nameserver %s] "+fmt, append([]interface{}{n.ourName}, args...)...)
}
