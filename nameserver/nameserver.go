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
	tombstoneTimeout = time.Minute * 10

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

func (n *Nameserver) AddEntry(hostname, containerid string, origin router.PeerName, addr address.Address) error {
	n.infof("adding entry %s -> %s", hostname, addr.String())
	n.Lock()
	entry := n.entries.add(hostname, containerid, origin, addr)
	n.Unlock()

	if n.gossip != nil {
		return n.gossip.GossipBroadcast(&Entries{entry})
	}
	return nil
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

func (n *Nameserver) ContainerDied(ident string) error {
	n.Lock()
	entries := n.entries.tombstone(n.ourName, func(e *Entry) bool {
		if e.ContainerID == ident {
			n.infof("container %s died, tombstoning entry %s", ident, e.String())
			return true
		}
		return false
	})
	n.Unlock()
	if n.gossip != nil {
		return n.gossip.GossipBroadcast(entries)
	}
	return nil
}

func (n *Nameserver) PeerGone(peer *router.Peer) {
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
		return true
	})
	n.Unlock()
	if n.gossip != nil {
		return n.gossip.GossipBroadcast(entries)
	}
	return nil
}

func (n *Nameserver) deleteTombstones() {
	n.Lock()
	defer n.Unlock()
	now := time.Now().Unix()
	n.entries.filter(func(e *Entry) bool {
		return now-e.Tombstone <= int64(tombstoneTimeout/time.Second)
	})
}

func (n *Nameserver) String() string {
	n.RLock()
	defer n.RUnlock()
	var buf bytes.Buffer
	for _, entry := range n.entries {
		if entry.Tombstone > 0 {
			continue
		}
		containerid := entry.ContainerID
		if len(containerid) > 12 {
			containerid = containerid[:12]
		}
		fmt.Fprintf(&buf, "%s: %s [%s]\n", containerid, entry.Hostname, entry.Addr.String())
	}
	return buf.String()
}

func (n *Nameserver) Gossip() router.GossipData {
	n.RLock()
	defer n.RUnlock()
	result := make(Entries, len(n.entries))
	copy(result, n.entries)
	return &result
}

func (n *Nameserver) OnGossipUnicast(sender router.PeerName, msg []byte) error {
	return nil
}

func (n *Nameserver) receiveGossip(msg []byte) (router.GossipData, router.GossipData, error) {
	var entries Entries
	if err := gob.NewDecoder(bytes.NewReader(msg)).Decode(&entries); err != nil {
		return nil, nil, err
	}

	if err := entries.check(); err != nil {
		return nil, nil, err
	}

	n.Lock()
	defer n.Unlock()

	if n.peers != nil {
		entries.filter(func(e *Entry) bool {
			return n.peers.Fetch(e.Origin) != nil
		})
	}

	newEntries := n.entries.merge(entries)
	return &newEntries, &entries, nil
}

// merge received data into state and return "everything new I've
// just learnt", or nil if nothing in the received data was new
func (n *Nameserver) OnGossip(msg []byte) (router.GossipData, error) {
	newEntries, _, err := n.receiveGossip(msg)
	return newEntries, err
}

// merge received data into state and return a representation of
// the received data, for further propagation
func (n *Nameserver) OnGossipBroadcast(msg []byte) (router.GossipData, error) {
	_, entries, err := n.receiveGossip(msg)
	return entries, err
}

func (n *Nameserver) infof(fmt string, args ...interface{}) {
	Log.Infof("[nameserver %s] "+fmt, append([]interface{}{n.ourName}, args...)...)
}
func (n *Nameserver) debugf(fmt string, args ...interface{}) {
	Log.Debugf("[nameserver %s] "+fmt, append([]interface{}{n.ourName}, args...)...)
}
