package nameserver

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"sync"
	"time"

	"github.com/miekg/dns"

	. "github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/common/docker"
	"github.com/weaveworks/weave/net/address"
	"github.com/weaveworks/weave/router"
)

const (
	// Tombstones do not need to survive long periods of peer disconnection, as
	// we delete entries for disconnected peers.  Therefore they just need to hang
	// around to account for propagation delay through gossip.  10 minutes sounds
	// long enough.
	tombstoneTimeout = time.Minute * 30

	// Used by prog/weaver/main.go and proxy/create_container_interceptor.go
	DefaultDomain = "weave.local."

	// Maximum age of acceptable gossip messages (to account for clock skew)
	gossipWindow = int64(tombstoneTimeout/time.Second) / 2
)

// Nameserver: gossip-based, in memory nameserver.
// - Holds a sorted list of (hostname, peer, container id, ip) tuples for the whole cluster.
// - This list is gossiped & merged around the cluser.
// - Lookup-by-hostname are O(nlogn), and return a (copy of a) slice of the entries
// - Update is O(n) for now
type Nameserver struct {
	sync.RWMutex
	ourName router.PeerName
	domain  string
	gossip  router.Gossip
	docker  *docker.Client
	entries Entries
	peers   *router.Peers
	quit    chan struct{}
}

func New(ourName router.PeerName, peers *router.Peers, docker *docker.Client, domain string) *Nameserver {
	ns := &Nameserver{
		ourName: ourName,
		domain:  dns.Fqdn(domain),
		peers:   peers,
		docker:  docker,
		quit:    make(chan struct{}),
	}
	if peers != nil {
		peers.OnGC(ns.PeerGone)
	}
	return ns
}

func (n *Nameserver) SetGossip(gossip router.Gossip) {
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
	if n.gossip != nil {
		return n.gossip.GossipBroadcast(&GossipData{
			Entries:   Entries(es),
			Timestamp: now(),
		})
	}
	return nil
}

func (n *Nameserver) AddEntry(hostname, containerid string, origin router.PeerName, addr address.Address) error {
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
		return e.Addr == ip
	})
	if err != nil {
		return "", err
	}
	n.debugf("reverse lookup %s -> %s", ip, match.Hostname)
	return match.Hostname, nil
}

func (n *Nameserver) ContainerDied(ident string) {
	n.Lock()
	entries := n.entries.tombstone(n.ourName, func(e *Entry) bool {
		if e.ContainerID == ident {
			n.infof("container %s died, tombstoning entry %s", ident, e.String())
			return true
		}
		return false
	})
	n.Unlock()
	if len(entries) > 0 {
		if err := n.broadcastEntries(entries...); err != nil {
			n.errorf("Failed to broadcast container '%s' death: %v", ident, err)
		}
	}
}

func (n *Nameserver) PeerGone(peer *router.Peer) {
	n.infof("peer gone %s", peer.String())
	n.Lock()
	defer n.Unlock()
	n.entries.filter(func(e *Entry) bool {
		return e.Origin != peer.Name
	})
}

func (n *Nameserver) Delete(hostname, containerid, ipStr string, ip address.Address) error {
	n.Lock()
	n.infof("tombstoning hostname=%s, containerid=%s, ip=%s", hostname, containerid, ipStr)
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

func (n *Nameserver) Gossip() router.GossipData {
	n.RLock()
	defer n.RUnlock()
	gossip := &GossipData{
		Entries:   make(Entries, len(n.entries)),
		Timestamp: now(),
	}
	copy(gossip.Entries, n.entries)
	return gossip
}

func (n *Nameserver) OnGossipUnicast(sender router.PeerName, msg []byte) error {
	return nil
}

func (n *Nameserver) receiveGossip(msg []byte) (router.GossipData, router.GossipData, error) {
	var gossip GossipData
	if err := gob.NewDecoder(bytes.NewReader(msg)).Decode(&gossip); err != nil {
		return nil, nil, err
	}

	if delta := gossip.Timestamp - now(); delta > gossipWindow || delta < -gossipWindow {
		return nil, nil, fmt.Errorf("clock skew of %d detected", delta)
	}

	if err := gossip.Entries.check(); err != nil {
		return nil, nil, err
	}

	n.Lock()
	defer n.Unlock()

	if n.peers != nil {
		gossip.Entries.filter(func(e *Entry) bool {
			return n.peers.Fetch(e.Origin) != nil
		})
	}

	newEntries := n.entries.merge(gossip.Entries)
	if len(newEntries) > 0 {
		return &GossipData{Entries: newEntries, Timestamp: now()}, &gossip, nil
	}
	return nil, &gossip, nil
}

// merge received data into state and return "everything new I've
// just learnt", or nil if nothing in the received data was new
func (n *Nameserver) OnGossip(msg []byte) (router.GossipData, error) {
	newEntries, _, err := n.receiveGossip(msg)
	return newEntries, err
}

// merge received data into state and return a representation of
// the received data, for further propagation
func (n *Nameserver) OnGossipBroadcast(_ router.PeerName, msg []byte) (router.GossipData, error) {
	_, entries, err := n.receiveGossip(msg)
	return entries, err
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
