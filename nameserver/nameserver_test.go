package nameserver

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"math/rand"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/weaveworks/mesh"

	"github.com/weaveworks/weave/net/address"
	"github.com/weaveworks/weave/testing/gossip"
)

const (
	container1 = "c1"
	container2 = "c2"
	container3 = "c3"
	container4 = "c4"
	container5 = "c5"
	container6 = "c6"
	hostname1  = "hostname1"
	hostname2  = "hostname2"
	hostname3  = "hostname3"
	hostname4  = "hostname4"
	hostname5  = "hostname5"
	hostname6  = "hostname6"
	addr1      = address.Address(1)
	addr2      = address.Address(2)
	addr3      = address.Address(3)
	addr4      = address.Address(4)
	addr5      = address.Address(5)
	addr6      = address.Address(6)
)

func makeNameserver(name mesh.PeerName) *Nameserver {
	return makeNameserverWithRestore(name, NewContainerIDSet(), NewContainerIDSet())
}

func makeNameserverWithRestore(name mesh.PeerName,
	nonStopped, stopped ContainerIDSet) *Nameserver {

	return New(name, "", NewMockDB(), func(mesh.PeerName) bool { return true },
		nonStopped, stopped)
}

func makeNetwork(size int) ([]*Nameserver, *gossip.TestRouter) {
	gossipRouter := gossip.NewTestRouter(0.0)
	nameservers := make([]*Nameserver, size)

	for i := 0; i < size; i++ {
		name, _ := mesh.PeerNameFromString(fmt.Sprintf("%02d:00:00:02:00:00", i))
		nameserver := makeNameserver(name)
		nameserver.SetGossip(gossipRouter.Connect(nameserver.ourName, nameserver))
		nameserver.Start()
		nameservers[i] = nameserver
	}

	return nameservers, gossipRouter
}

func stopNetwork(nameservers []*Nameserver, grouter *gossip.TestRouter) {
	for _, nameserver := range nameservers {
		nameserver.Stop()
	}
	grouter.Stop()
}

type pair struct {
	origin mesh.PeerName
	addr   address.Address
}

type mapping struct {
	hostname string
	addrs    []pair
}

type addrs []address.Address

func (a addrs) Len() int           { return len(a) }
func (a addrs) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a addrs) Less(i, j int) bool { return a[i] < a[j] }
func (a addrs) String() string {
	ss := []string{}
	for _, addr := range a {
		ss = append(ss, addr.String())
	}
	return strings.Join(ss, " ")
}

func (m mapping) Addrs() []address.Address {
	want := addrs{}
	for _, p := range m.addrs {
		want = append(want, p.addr)
	}
	sort.Sort(want)
	return want
}

func (n *Nameserver) copyEntries() Entries {
	n.RLock()
	defer n.RUnlock()

	entries := make(Entries, len(n.entries))
	copy(entries, n.entries)
	return entries
}

func (es Entries) unsetStopped() {
	for i := range es {
		es[i].stopped = false
	}
}

// Database mock

type MockDB struct {
	data map[string][]byte
}

func NewMockDB() *MockDB {
	return &MockDB{data: make(map[string][]byte)}
}

func (m *MockDB) Load(key string, val interface{}) (bool, error) {
	if ret, ok := m.data[key]; ok {
		reader := bytes.NewReader(ret)
		dec := gob.NewDecoder(reader)
		err := dec.Decode(val)
		return true, err
	}
	return false, nil
}

func (m *MockDB) Save(key string, val interface{}) error {
	buf := bytes.NewBuffer(nil)
	enc := gob.NewEncoder(buf)
	if err := enc.Encode(val); err != nil {
		return err
	}
	m.data[key] = buf.Bytes()
	return nil
}

func TestNameservers(t *testing.T) {
	//common.SetLogLevel("debug")

	lookupTimeout := 10 // ms
	nameservers, grouter := makeNetwork(30)
	defer stopNetwork(nameservers, grouter)
	// This subset will sometimes lose touch with some of the others
	badNameservers := nameservers[25:]
	// This subset will remain well-connected, and we will deal mainly with them
	nameservers = nameservers[:25]
	nameserversByName := map[mesh.PeerName]*Nameserver{}
	for _, n := range nameservers {
		nameserversByName[n.ourName] = n
	}
	mappings := []mapping{}

	check := func(nameserver *Nameserver, expected mapping) {
		have := []address.Address{}
		for i := 0; i < lookupTimeout; i++ {
			have = nameserver.Lookup(expected.hostname)
			sort.Sort(addrs(have))
			if reflect.DeepEqual(have, expected.Addrs()) {
				return
			}
			time.Sleep(1 * time.Millisecond)
		}
		want := expected.Addrs()
		require.Equal(t, addrs(want).String(), addrs(have).String())
	}

	addMapping := func() {
		nameserver := nameservers[rand.Intn(len(nameservers))]
		addr := address.Address(rand.Int31())
		// Create a hostname which has some upper and lowercase letters,
		// and a unique number so we don't have to check if we allocated it already
		randomBits := rand.Int63()
		firstLetter := 'H' + (randomBits&1)*32
		secondLetter := 'O' + (randomBits&2)*16
		randomBits = randomBits >> 2
		hostname := fmt.Sprintf("%c%cstname%d", firstLetter, secondLetter, randomBits)
		mapping := mapping{hostname, []pair{{nameserver.ourName, addr}}}
		mappings = append(mappings, mapping)

		nameserver.AddEntry(hostname, "", nameserver.ourName, addr)
		check(nameserver, mapping)
	}

	addExtraMapping := func() {
		if len(mappings) <= 0 {
			return
		}
		nameserver := nameservers[rand.Intn(len(nameservers))]
		i := rand.Intn(len(mappings))
		mapping := mappings[i]
		addr := address.Address(rand.Int31())
		mapping.addrs = append(mapping.addrs, pair{nameserver.ourName, addr})
		mappings[i] = mapping

		nameserver.AddEntry(mapping.hostname, "", nameserver.ourName, addr)
		check(nameserver, mapping)
	}

	loseConnection := func() {
		nameserver1 := badNameservers[rand.Intn(len(badNameservers))]
		nameserver2 := nameservers[rand.Intn(len(nameservers))]
		nameserver1.PeerGone(nameserver2.ourName)
	}

	deleteMapping := func() {
		if len(mappings) <= 0 {
			return
		}
		i := rand.Intn(len(mappings))
		mapping := mappings[i]
		if len(mapping.addrs) <= 0 {
			return
		}
		j := rand.Intn(len(mapping.addrs))
		pair := mapping.addrs[j]
		mapping.addrs = append(mapping.addrs[:j], mapping.addrs[j+1:]...)
		mappings[i] = mapping
		nameserver := nameserversByName[pair.origin]

		nameserver.Delete(mapping.hostname, "*", pair.addr.String(), pair.addr)
		check(nameserver, mapping)
	}

	doLookup := func() {
		if len(mappings) <= 0 {
			return
		}
		mapping := mappings[rand.Intn(len(mappings))]
		nameserver := nameservers[rand.Intn(len(nameservers))]
		check(nameserver, mapping)
	}

	doReverseLookup := func() {
		if len(mappings) <= 0 {
			return
		}
		mapping := mappings[rand.Intn(len(mappings))]
		if len(mapping.addrs) <= 0 {
			return
		}
		nameserver := nameservers[rand.Intn(len(nameservers))]
		hostname := ""
		var err error
		for i := 0; i < lookupTimeout; i++ {
			hostname, err = nameserver.ReverseLookup(mapping.addrs[0].addr)
			if err != nil && mapping.hostname == hostname {
				return
			}
			time.Sleep(1 * time.Millisecond)
		}
		require.Nil(t, err)
		require.Equal(t, mapping.hostname, hostname)
	}

	for i := 0; i < 800; i++ {
		r := rand.Float32()
		switch {
		case r < 0.1:
			addMapping()

		case 0.1 <= r && r < 0.2:
			addExtraMapping()

		case 0.2 <= r && r < 0.3:
			deleteMapping()

		case 0.3 <= r && r < 0.35:
			loseConnection()

		case 0.35 <= r && r < 0.9:
			doLookup()

		case 0.9 <= r:
			doReverseLookup()
		}

		grouter.Flush()
	}
}

func TestContainerAndPeerDeath(t *testing.T) {
	peername, err := mesh.PeerNameFromString("00:00:00:02:00:00")
	require.Nil(t, err)
	nameserver := makeNameserver(peername)

	nameserver.AddEntry(hostname1, container1, peername, addr1)
	require.Equal(t, []address.Address{addr1}, nameserver.Lookup(hostname1))

	nameserver.ContainerDied(container1)
	require.Equal(t, []address.Address{}, nameserver.Lookup(hostname1))

	nameserver.AddEntry(hostname1, container1, peername, addr1)
	require.Equal(t, []address.Address{addr1}, nameserver.Lookup(hostname1))

	nameserver.PeerGone(peername)
	require.Equal(t, []address.Address{}, nameserver.Lookup(hostname1))
}

func TestTombstoneDeletion(t *testing.T) {
	oldNow := now
	defer func() { now = oldNow }()
	now = func() int64 { return 1234 }

	peername, err := mesh.PeerNameFromString("00:00:00:02:00:00")
	require.Nil(t, err)
	nameserver := makeNameserver(peername)

	nameserver.AddEntry(hostname1, container1, peername, addr1)
	require.Equal(t, []address.Address{addr1}, nameserver.Lookup(hostname1))

	nameserver.deleteTombstones()
	require.Equal(t, []address.Address{addr1}, nameserver.Lookup(hostname1))

	nameserver.Delete(hostname1, container1, "", addr1)
	require.Equal(t, []address.Address{}, nameserver.Lookup(hostname1))
	require.Equal(t, l(Entries{Entry{
		ContainerID: container1,
		Origin:      peername,
		Addr:        addr1,
		Hostname:    hostname1,
		Version:     1,
		Tombstone:   1234,
	}}), nameserver.entries)

	now = func() int64 { return 1234 + int64(tombstoneTimeout/time.Second) + 1 }
	nameserver.deleteTombstones()
	require.Equal(t, Entries{}, nameserver.entries)
}

// TestRestore tests whether entries have been restored after the nameserver restart.
func TestRestore(t *testing.T) {
	oldNow := now
	defer func() { now = oldNow }()
	now = func() int64 { return 1000 }

	nameservers, grouter := makeNetwork(2)
	defer stopNetwork(nameservers, grouter)
	defer grouter.Flush()
	ns1, ns2 := nameservers[0], nameservers[1]

	nonStopped := NewContainerIDSet()
	stopped := NewContainerIDSet()

	// Container NonStopped | Entry Stopped
	ns1.AddEntry(hostname1, container1, ns1.ourName, addr1)
	ns1.ContainerDied(container1)
	nonStopped[container1] = struct{}{}
	// Container Stopped | Entry OK
	ns1.AddEntry(hostname2, container2, ns1.ourName, addr2)
	stopped[container2] = struct{}{}
	// Container Removed | Entry Stopped
	ns1.AddEntry(hostname3, container3, ns1.ourName, addr3)
	ns1.ContainerDied(container3)
	// Container Removed | Entry OK
	ns1.AddEntry(hostname4, container4, ns1.ourName, addr4)
	// Container NonStopped | Entry OK
	ns1.AddEntry(hostname5, container5, ns1.ourName, addr5)
	nonStopped[container5] = struct{}{}
	ns2.AddEntry(hostname6, container6, ns2.ourName, addr6)

	// Restart ns1
	ns1.Stop()
	now = func() int64 { return 1500 }
	grouter.RemovePeer(ns1.ourName)
	nameservers[0] = New(ns1.ourName, "", ns1.db, func(mesh.PeerName) bool { return true },
		nonStopped, stopped)
	ns1 = nameservers[0]
	ns1.SetGossip(grouter.Connect(ns1.ourName, ns1))
	ns1.Start()

	expected := l(Entries{
		Entry{
			ContainerID: container1,
			Origin:      ns1.ourName,
			Addr:        addr1,
			Hostname:    hostname1,
			Version:     2,
			Tombstone:   0,
			stopped:     false,
		},
		Entry{
			ContainerID: container2,
			Origin:      ns1.ourName,
			Addr:        addr2,
			Hostname:    hostname2,
			Version:     1,
			Tombstone:   1500,
			stopped:     true,
		},
		Entry{
			ContainerID: container3,
			Origin:      ns1.ourName,
			Addr:        addr3,
			Hostname:    hostname3,
			Version:     2,
			Tombstone:   1000,
			stopped:     false,
		},
		Entry{
			ContainerID: container4,
			Origin:      ns1.ourName,
			Addr:        addr4,
			Hostname:    hostname4,
			Version:     1,
			Tombstone:   1500,
			stopped:     false,
		},
		Entry{
			ContainerID: container5,
			Origin:      ns1.ourName,
			Addr:        addr5,
			Hostname:    hostname5,
			Version:     0,
			Tombstone:   0,
			stopped:     false,
		},
		Entry{
			ContainerID: container6,
			Origin:      ns2.ourName,
			Addr:        addr6,
			Hostname:    hostname6,
			Version:     0,
			Tombstone:   0,
			stopped:     false,
		},
	})

	time.Sleep(200 * time.Millisecond)
	// TODO(mp) Check why ns2 entries are not broadcasted to ns1
	require.Equal(t, expected[:5], ns1.copyEntries()[:5])
	expected.unsetStopped()
	require.Equal(t, expected, ns2.copyEntries())
}

// AddEntry, ContainerDied, ContainerDestroyed
func TestContainerEvents(t *testing.T) {
	// TODO(mp)
}

// TestAddEntryWithRestore tests whether stopped entries have been restored and
// broadcasted to the connected peers.
func TestAddEntryWithRestore(t *testing.T) {
	oldNow := now
	defer func() { now = oldNow }()
	now = func() int64 { return 1234 }

	nameservers, grouter := makeNetwork(2)
	defer stopNetwork(nameservers, grouter)
	ns1, ns2 := nameservers[0], nameservers[1]

	ns1.AddEntry(hostname2, container1, ns1.ourName, addr1)
	ns1.AddEntry(hostname3, container2, ns1.ourName, addr2)
	ns1.Delete(hostname3, container2, "", addr2)

	// Restart ns1 and preserve its db instance
	time.Sleep(200 * time.Millisecond)
	ns1.Stop()
	grouter.RemovePeer(ns1.ourName)
	ns2.PeerGone(ns1.ourName)
	nonStopped := NewContainerIDSet()
	stopped := NewContainerIDSet(container1)
	nameservers[0] = New(ns1.ourName, "", ns1.db, func(mesh.PeerName) bool { return true },
		nonStopped, stopped)
	ns1 = nameservers[0]
	ns1.SetGossip(grouter.Connect(ns1.ourName, ns1))
	ns1.Start()

	// At this point, the c1 entry is set to stopped
	entries := ns1.copyEntries()
	require.Equal(t, l(Entries{
		Entry{
			ContainerID: container1,
			Origin:      ns1.ourName,
			Addr:        addr1,
			Hostname:    hostname2,
			Version:     1,
			Tombstone:   1234,
			stopped:     true,
		},
		Entry{
			ContainerID: container2,
			Origin:      ns1.ourName,
			Addr:        addr2,
			Hostname:    hostname3,
			Version:     1,
			Tombstone:   1234,
			stopped:     false,
		}}), entries)

	time.Sleep(200 * time.Millisecond)
	entries = ns2.copyEntries()
	require.Equal(t, l(Entries{
		Entry{
			ContainerID: container1,
			Origin:      ns1.ourName,
			Addr:        addr1,
			Hostname:    hostname2,
			Version:     1,
			Tombstone:   1234,
			stopped:     false,
		},
		Entry{
			ContainerID: container2,
			Origin:      ns1.ourName,
			Addr:        addr2,
			Hostname:    hostname3,
			Version:     1,
			Tombstone:   1234,
			stopped:     false,
		}}), entries)

	ns1.AddEntry(hostname1, container1, ns1.ourName, addr3)
	time.Sleep(200 * time.Millisecond)

	// c1 (hostname1 -> addr1) should be restored and propagated to ns2
	entries = ns2.copyEntries()
	require.Len(t, entries, 3)
	require.Equal(t, l(Entries{
		Entry{
			ContainerID: container1,
			Origin:      ns1.ourName,
			Addr:        addr3,
			Hostname:    hostname1,
			Version:     0,
			Tombstone:   0,
			stopped:     false,
		},
		Entry{
			ContainerID: container1,
			Origin:      ns1.ourName,
			Addr:        addr1,
			Hostname:    hostname2,
			Version:     2,
			Tombstone:   0,
			stopped:     false,
		},
		Entry{
			ContainerID: container2,
			Origin:      ns1.ourName,
			Addr:        addr2,
			Hostname:    hostname3,
			Version:     1,
			Tombstone:   1234,
			stopped:     false,
		}}), entries)

	grouter.Flush()
}

// TestStopContainer tests whether:
// * AddEntry restores entries.
// * ContainerDied sets stop flag.
// * ContainerDestroyed unsets the flag.
func TestStopContainer(t *testing.T) {
	oldNow := now
	defer func() { now = oldNow }()
	now = func() int64 { return 1234 }

	name, _ := mesh.PeerNameFromString("00:00:00:02:00:00")
	nameserver := makeNameserver(name)

	nameserver.AddEntry(hostname1, container1, name, addr1)
	nameserver.AddEntry(hostname2, container1, name, addr1)
	nameserver.AddEntry(hostname3, container3, name, addr3)
	require.Equal(t, l(Entries{
		Entry{
			ContainerID: container1,
			Origin:      nameserver.ourName,
			Addr:        addr1,
			Hostname:    hostname1,
			Version:     0,
			Tombstone:   0,
			stopped:     false,
		},
		Entry{
			ContainerID: container1,
			Origin:      nameserver.ourName,
			Addr:        addr1,
			Hostname:    hostname2,
			Version:     0,
			Tombstone:   0,
			stopped:     false,
		},
		Entry{
			ContainerID: container3,
			Origin:      nameserver.ourName,
			Addr:        addr3,
			Hostname:    hostname3,
			Version:     0,
			Tombstone:   0,
			stopped:     false,
		}}), nameserver.copyEntries())

	nameserver.Delete(hostname1, container1, "", addr1)
	nameserver.ContainerDied(container1)
	require.Equal(t, l(Entries{
		Entry{
			ContainerID: container1,
			Origin:      nameserver.ourName,
			Addr:        addr1,
			Hostname:    hostname1,
			Version:     1,
			Tombstone:   1234,
			stopped:     false,
		},
		Entry{
			ContainerID: container1,
			Origin:      nameserver.ourName,
			Addr:        addr1,
			Hostname:    hostname2,
			Version:     1,
			Tombstone:   1234,
			stopped:     true,
		},
		Entry{
			ContainerID: container3,
			Origin:      nameserver.ourName,
			Addr:        addr3,
			Hostname:    hostname3,
			Version:     0,
			Tombstone:   0,
			stopped:     false,
		}}), nameserver.copyEntries())

	// AddEntry should restore the non-tombstoned entry
	now = func() int64 { return 1235 }
	nameserver.AddEntry(hostname2, container1, name, addr1)
	require.Equal(t, l(Entries{
		Entry{
			ContainerID: container1,
			Origin:      nameserver.ourName,
			Addr:        addr1,
			Hostname:    hostname1,
			Version:     1,
			Tombstone:   1234,
			stopped:     false,
		},
		Entry{
			ContainerID: container1,
			Origin:      nameserver.ourName,
			Addr:        addr1,
			Hostname:    hostname2,
			Version:     2,
			Tombstone:   0,
			stopped:     false,
		},
		Entry{
			ContainerID: container3,
			Origin:      nameserver.ourName,
			Addr:        addr3,
			Hostname:    hostname3,
			Version:     0,
			Tombstone:   0,
			stopped:     false,
		}}), nameserver.copyEntries())

	now = func() int64 { return 1236 }
	// All container1 entries should be tombstoned
	nameserver.ContainerDestroyed(container1)
	require.Equal(t, l(Entries{
		Entry{
			ContainerID: container1,
			Origin:      nameserver.ourName,
			Addr:        addr1,
			Hostname:    hostname1,
			Version:     1,
			Tombstone:   1234,
			stopped:     false,
		},
		Entry{
			ContainerID: container1,
			Origin:      nameserver.ourName,
			Addr:        addr1,
			Hostname:    hostname2,
			Version:     3,
			Tombstone:   1236,
			stopped:     false,
		},
		Entry{
			ContainerID: container3,
			Origin:      nameserver.ourName,
			Addr:        addr3,
			Hostname:    hostname3,
			Version:     0,
			Tombstone:   0,
			stopped:     false,
		}}), nameserver.copyEntries())
}
