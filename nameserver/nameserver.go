package nameserver

import (
	"fmt"
	"sync"
	"time"

	"github.com/miekg/dns"
	"github.com/weaveworks/mesh"

	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/db"
	"github.com/weaveworks/weave/net/address"
)

// TODO(mp) double-check merging

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

	// Used as a key for stored nameserver entries in the database
	nameserverIdent = "nameserver"
)

// Nameserver: gossip-based, in memory nameserver.
// - Holds a sorted list of (hostname, peer, container id, ip) tuples for the whole cluster.
// - DB handle for persisting local entries to survive container/peer restarts.
// - This list is gossiped & merged around the cluser.
// - Lookup-by-hostname are O(nlogn), and return a (copy of a) slice of the entries
// - Update is O(n) for now
type Nameserver struct {
	sync.RWMutex
	ourName          mesh.PeerName
	domain           string
	gossip           mesh.Gossip
	entries          Entries
	isKnownPeer      func(mesh.PeerName) bool
	quit             chan struct{}
	db               db.DB
	pendingBroadcast Entries
}

// dbEntry is used when storing each entry to DB.
type dbEntry struct {
	Entry
	Stopped bool
}

type set map[string]struct{}

func newSet(items []string) set {
	s := set{}
	for _, i := range items {
		s[i] = struct{}{}
	}
	return s
}

func (s set) exist(item string) bool {
	_, ok := s[item]
	return ok
}

// TODO(mp) comment
func New(ourName mesh.PeerName, domain string, db db.DB,
	isKnownPeer func(mesh.PeerName) bool, nonStopped, stopped []string) *Nameserver {

	n := &Nameserver{
		ourName:     ourName,
		domain:      dns.Fqdn(domain),
		db:          db,
		isKnownPeer: isKnownPeer,
		quit:        make(chan struct{}),
	}
	n.restoreFromDB(nonStopped, stopped)

	return n
}

// Restores local entries from the database.
// NB: called only during the initialization, thus the lock is not taken.
func (n *Nameserver) restoreFromDB(nonStopped, stopped []string) {
	n.debugf("restore: starting...")

	// TODO(mp) check race
	//n.debugf("restore: sleeping...")
	//time.Sleep(60 * time.Second)
	//n.debugf("restore: sleeping done.")

	if ok := n.loadEntries(); !ok {
		return
	}

	// TODO(mp) maybe just replace with sort.Search...
	nonStoppedSet := newSet(nonStopped)
	stoppedSet := newSet(stopped)

	// <braindump>
	// The IDEA:
	// * During restoration, .stopped is set only for containers which are not running;
	//   for others, we just restore entries aka restore entries from
	// * ContainerDied: we set .stopped
	// * ContainerDestroyed: we remove all container entries
	// * AddEntry: restore stopped entries (weave:extern non-tombstoned will be stopped, which we restore later)
	// * (maybe) Move restoration from AddEntry to ContainerStarted
	// * Possible race: AllContainers <- gap (start new container) -> restoreFromDB (maybe not, if we do not allow to
	// start while being in process of the initialization)
	// </braindump>

	// ------------------+-------------------+-------------------+---------------+------------
	// Container NonStopped | Container Stopped |  Entry Tombstoned | Entry Stopped | Action
	// ---------------------+-------------------+-------------------+---------------+------------
	//           1          |        0          |        1          |       1       | Restore
	//           1          |        0          |        1          |       0       | noop
	//           1          |        0          |        0          |       0       | noop
	//           0          |        1          |        1          |       1       | noop
	//           0          |        1          |        1          |       0       | noop
	//           0          |        1          |        0          |       0       | Stop
	//           0          |        0          |        1          |       1       | Tombstone
	//           0          |        0          |        1          |       0       | noop
	//           0          |        0          |        0          |       0       | Tombstone
	// ---------------------+-------------------+-------------------+---------------+------------
	// * (Container NonStopped) OR (Container Stopped) == 0 => Container has been removed.

	now := now()
	for i, e := range n.entries {
		if e.Origin != n.ourName {
			panic(fmt.Sprintf(
				"restore: peer %s DB should not have contained entries of %s peer",
				n.ourName, e.Origin))
		}

		switch {
		case nonStoppedSet.exist(e.ContainerID) && e.stopped:
			n.debugf("restore: restoring %v...", e)
			e.stopped = false
			e.Tombstone = 0
			e.Version++
		case stoppedSet.exist(e.ContainerID) && e.Tombstone == 0:
			n.debugf("restore: stopping %v...", e)
			e.stopped = true
			e.Tombstone = now
			e.Version++
		case !nonStoppedSet.exist(e.ContainerID) && !stoppedSet.exist(e.ContainerID) && e.stopped:
			n.debugf("restore: removing %v...", e)
			e.stopped = false
			e.Version++
		case !nonStoppedSet.exist(e.ContainerID) && !stoppedSet.exist(e.ContainerID) && e.Tombstone == 0:
			n.debugf("restore: tombstoning %v...", e)
			e.stopped = false
			e.Tombstone = now
			e.Version++
		default:
			n.debugf("restore: restoring %v...", e)
		}
		n.entries[i] = e
	}

	// lHostname gets lost during serialization, so we need to set this field manually
	n.entries.addLowercase()
	n.snapshot()

	// Will gossip once we set the gossiper
	n.pendingBroadcast = make(Entries, len(n.entries))
	copy(n.pendingBroadcast, n.entries)

	n.debugf("restore: finished.")
}

// NB: Can be called only before n.Start()
func (n *Nameserver) SetGossip(gossip mesh.Gossip) {
	n.gossip = gossip
	n.broadcastEntries(n.pendingBroadcast...)
	n.pendingBroadcast = nil
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

func (n *Nameserver) broadcastEntries(es ...Entry) {
	if n.gossip == nil || len(es) == 0 {
		return
	}
	n.gossip.GossipBroadcast(&GossipData{
		Entries:   Entries(es),
		Timestamp: now(),
	})
}

func (n *Nameserver) AddEntry(hostname, containerid string,
	origin mesh.PeerName, addr address.Address) {

	var entries Entries
	var found bool

	n.Lock()

	// Restore the stopped entries first
	for i, e := range n.entries {
		if e.Origin == origin && e.ContainerID == containerid && e.stopped {
			if !found && e.Addr == addr {
				found = true
			}
			e.stopped = false
			e.Tombstone = 0
			e.Version++
			n.entries[i] = e
			entries = append(entries, e)
		}
	}

	if !found {
		entries = append(entries, n.entries.add(hostname, containerid, origin, addr))
	}

	n.snapshot()
	n.Unlock()
	n.broadcastEntries(entries...)
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

func (n *Nameserver) ContainerStarted(ident string) {
	// TODO(mp) maybe move code from AddEntry here
}
func (n *Nameserver) ContainerDestroyed(ident string) {
	n.Lock()
	entries := n.entries.forceTombstone(n.ourName, func(e *Entry) bool {
		if e.ContainerID == ident {
			n.infof("container %s destroyed; tombstoning entry %s", ident, e.String())
			// Unset the flag that the entry could be GC later on
			e.stopped = false
			return true
		}
		return false
	})
	n.snapshot()
	n.Unlock()
	n.broadcastEntries(entries...)
}

func (n *Nameserver) ContainerDied(ident string) {
	n.Lock()
	entries := n.entries.tombstone(n.ourName, func(e *Entry) bool {
		if e.ContainerID == ident {
			n.infof("container %s died; tombstoning entry %s", ident, e.String())
			// We might want to restore the entry if container comes back
			e.stopped = true
			return true
		}
		return false
	})
	n.snapshot()
	n.Unlock()
	n.broadcastEntries(entries...)
}

func (n *Nameserver) PeerGone(peer mesh.PeerName) {
	n.infof("peer %s gone", peer.String())
	n.Lock()
	defer n.Unlock()
	n.entries.filter(func(e *Entry) bool {
		return e.Origin != peer
	})
}

func (n *Nameserver) Delete(hostname, containerid, ipStr string, ip address.Address) {
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
	n.snapshot()
	n.Unlock()
	n.broadcastEntries(entries...)
}

func (n *Nameserver) deleteTombstones() {
	n.Lock()
	defer n.Unlock()
	now := time.Now().Unix()
	n.entries.filter(func(e *Entry) bool {
		return e.Tombstone == 0 || e.stopped || now-e.Tombstone <= int64(tombstoneTimeout/time.Second)
	})
	n.snapshot()
}

// snapshot stores the local entries (Origin == n.ourName) to the database.
// Each entry is wrapped by the dbEntry struct because the entry.stopped field is private
// and is lost during the serialization performed by the db.
//
// NB: we assume that a caller has taken the nameserver' lock for reading or writing.
func (n *Nameserver) snapshot() {
	var dbEntries []dbEntry

	for _, e := range n.entries {
		if e.Origin == n.ourName {
			dbEntries = append(dbEntries, dbEntry{e, e.stopped})
		}
	}

	if err := n.db.Save(nameserverIdent, dbEntries); err != nil {
		n.errorf("saving to DB failed due to: %s", err)
	}
}

// loadEntries loads and unwraps entries from DB.
func (n *Nameserver) loadEntries() bool {
	var dbEntries []dbEntry

	ok, err := n.db.Load(nameserverIdent, &dbEntries)
	switch {
	case err != nil:
		n.errorf("restore: cannot retrieve entries due to: %s", err)
		return false
	case !ok:
		n.infof("restore: cannot find any entry in DB")
		return false
	}

	for _, e := range dbEntries {
		entry := e.Entry
		entry.stopped = e.Stopped
		n.entries = append(n.entries, entry)
	}

	return true
}

func (n *Nameserver) Gossip() mesh.GossipData {
	n.RLock()
	defer n.RUnlock()
	gossip := &GossipData{
		Entries:   make(Entries, len(n.entries)),
		Timestamp: now(),
	}
	copy(gossip.Entries, n.entries)
	return gossip
}

func (n *Nameserver) OnGossipUnicast(sender mesh.PeerName, msg []byte) error {
	return nil
}

func (n *Nameserver) receiveGossip(msg []byte) (mesh.GossipData, mesh.GossipData, error) {
	var gossip GossipData
	if err := gossip.Decode(msg); err != nil {
		return nil, nil, err
	}
	if delta := gossip.Timestamp - now(); delta > gossipWindow || delta < -gossipWindow {
		return nil, nil, fmt.Errorf("clock skew of %d detected", delta)
	}

	// Filter to remove entries from unknown peers, done before we take
	// the nameserver lock, so we don't have to worry what isKnownPeer locks.
	gossip.Entries.filter(func(e *Entry) bool {
		return n.isKnownPeer(e.Origin)
	})

	var overriddenEntries []Entry
	n.Lock()

	// Check entries claiming to originate from us against our current data
	gossip.Entries.filter(func(e *Entry) bool {
		if e.Origin == n.ourName {
			if ourEntry, ok := n.entries.findEqual(e); ok {
				if ourEntry.Version < e.Version ||
					(ourEntry.Version == e.Version && ourEntry.Tombstone != e.Tombstone) {
					// Take our version of the data, but make the version higher than the incoming
					nextVersion := e.Version + 1
					*e = *ourEntry
					e.Version = nextVersion
					overriddenEntries = append(overriddenEntries, *e)
				}
			} else { // We have no entry matching the one that came in with us as Origin
				if e.tombstone() {
					overriddenEntries = append(overriddenEntries, *e)
				}
			}
		}
		return true
	})

	newEntries := n.entries.merge(gossip.Entries)
	n.snapshot()
	n.Unlock() // unlock before attempting to broadcast

	// Note that all overriddenEntries have been merged into our entries, either
	// because we forced the version higher or because they were missing before.
	n.broadcastEntries(overriddenEntries...)

	if len(newEntries) > 0 {
		return &GossipData{Entries: newEntries, Timestamp: now()}, &gossip, nil
	}
	return nil, &gossip, nil
}

// merge received data into state and return "everything new I've
// just learnt", or nil if nothing in the received data was new
func (n *Nameserver) OnGossip(msg []byte) (mesh.GossipData, error) {
	newEntries, _, err := n.receiveGossip(msg)
	return newEntries, err
}

// merge received data into state and return a representation of
// the received data, for further propagation
func (n *Nameserver) OnGossipBroadcast(_ mesh.PeerName, msg []byte) (mesh.GossipData, error) {
	_, entries, err := n.receiveGossip(msg)
	return entries, err
}

// Logging

func (n *Nameserver) infof(fmt string, args ...interface{}) {
	n.logf(common.Log.Infof, fmt, args...)
}
func (n *Nameserver) debugf(fmt string, args ...interface{}) {
	n.logf(common.Log.Debugf, fmt, args...)
}
func (n *Nameserver) errorf(fmt string, args ...interface{}) {
	n.logf(common.Log.Errorf, fmt, args...)
}
func (n *Nameserver) logf(f func(string, ...interface{}), fmt string, args ...interface{}) {
	f("[nameserver %s] "+fmt, append([]interface{}{n.ourName}, args...)...)
}
