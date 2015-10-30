package nameserver

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/weaveworks/weave/net/address"
	"github.com/weaveworks/weave/router"
)

func makeEntries(values string) Entries {
	entries := make(Entries, len(values))
	for i, c := range values {
		entries[i] = Entry{Hostname: string(c)}
	}
	return entries
}

func TestAdd(t *testing.T) {
	oldNow := now
	defer func() { now = oldNow }()
	now = func() int64 { return 1234 }

	entries := Entries{}
	entries.add("A", "", router.UnknownPeerName, address.Address(0))
	expected := Entries{
		Entry{Hostname: "A", Origin: router.UnknownPeerName, Addr: address.Address(0)},
	}
	require.Equal(t, entries, expected)

	entries.tombstone(router.UnknownPeerName, func(e *Entry) bool { return e.Hostname == "A" })
	expected = Entries{
		Entry{Hostname: "A", Origin: router.UnknownPeerName, Addr: address.Address(0), Version: 1, Tombstone: 1234},
	}
	require.Equal(t, entries, expected)

	entries.add("A", "", router.UnknownPeerName, address.Address(0))
	expected = Entries{
		Entry{Hostname: "A", Origin: router.UnknownPeerName, Addr: address.Address(0), Version: 2},
	}
	require.Equal(t, entries, expected)
}

func TestMerge(t *testing.T) {
	e1 := makeEntries("ACDF")
	e2 := makeEntries("BEF")

	diff := e1.merge(e2)

	require.Equal(t, makeEntries("BE"), diff)
	require.Equal(t, makeEntries("ABCDEF"), e1)

	diff = e1.merge(e1)
	require.Equal(t, Entries{}, diff)
}

func TestOldMerge(t *testing.T) {
	e1 := Entries{Entry{Hostname: "A", Version: 0}}
	diff := e1.merge(Entries{Entry{Hostname: "A", Version: 1}})
	require.Equal(t, Entries{Entry{Hostname: "A", Version: 1}}, diff)
	require.Equal(t, Entries{Entry{Hostname: "A", Version: 1}}, e1)

	diff = e1.merge(Entries{Entry{Hostname: "A", Version: 0}})
	require.Equal(t, Entries{}, diff)
	require.Equal(t, Entries{Entry{Hostname: "A", Version: 1}}, e1)
}

func TestTombstone(t *testing.T) {
	oldNow := now
	defer func() { now = oldNow }()
	now = func() int64 { return 1234 }

	es := makeEntries("AB")

	es.tombstone(router.UnknownPeerName, func(e *Entry) bool {
		return e.Hostname == "B"
	})
	expected := Entries{
		Entry{Hostname: "A"},
		Entry{Hostname: "B", Version: 1, Tombstone: 1234},
	}
	require.Equal(t, expected, es)
}

func TestDelete(t *testing.T) {
	es := makeEntries("AB")

	es.filter(func(e *Entry) bool {
		return e.Hostname != "A"
	})
	require.Equal(t, makeEntries("B"), es)
}

func TestLookup(t *testing.T) {
	es := Entries{
		Entry{Hostname: "A"},
		Entry{Hostname: "B", ContainerID: "bar"},
		Entry{Hostname: "B", ContainerID: "foo"},
		Entry{Hostname: "C"},
	}

	have := es.lookup("B")
	want := Entries{
		Entry{Hostname: "B", ContainerID: "bar"},
		Entry{Hostname: "B", ContainerID: "foo"},
	}
	require.Equal(t, have, want)
}

func TestGossipDataMerge(t *testing.T) {
	g1 := GossipData{Entries: makeEntries("AcDf")}
	g2 := GossipData{Entries: makeEntries("BEf")}

	g1.Merge(&g2)

	require.Equal(t, GossipData{Entries: makeEntries("ABcDEf")}, g1)
}
