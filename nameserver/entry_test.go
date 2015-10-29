package nameserver

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/weaveworks/weave/net/address"
	"github.com/weaveworks/weave/router"
)

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
	e1 := Entries{
		Entry{Hostname: "A"},
		Entry{Hostname: "C"},
		Entry{Hostname: "D"},
		Entry{Hostname: "F"},
	}

	e2 := Entries{
		Entry{Hostname: "B"},
		Entry{Hostname: "E"},
		Entry{Hostname: "F"},
	}

	diff := e1.merge(e2)
	expectedDiff := Entries{
		Entry{Hostname: "B"},
		Entry{Hostname: "E"},
	}
	require.Equal(t, expectedDiff, diff)

	expected := Entries{
		Entry{Hostname: "A"},
		Entry{Hostname: "B"},
		Entry{Hostname: "C"},
		Entry{Hostname: "D"},
		Entry{Hostname: "E"},
		Entry{Hostname: "F"},
	}
	require.Equal(t, expected, e1)

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

	es := Entries{
		Entry{Hostname: "A"},
		Entry{Hostname: "B"},
	}

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
	es := Entries{
		Entry{Hostname: "A"},
		Entry{Hostname: "B"},
	}

	es.filter(func(e *Entry) bool {
		return e.Hostname != "A"
	})
	expected := Entries{
		Entry{Hostname: "B"},
	}
	require.Equal(t, expected, es)
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
	g1 := GossipData{Entries: Entries{
		Entry{Hostname: "A"},
		Entry{Hostname: "c"},
		Entry{Hostname: "D"},
		Entry{Hostname: "f"},
	}}

	g2 := GossipData{Entries: Entries{
		Entry{Hostname: "B"},
		Entry{Hostname: "E"},
		Entry{Hostname: "f"},
	}}

	g1.Merge(&g2)

	expected := GossipData{Entries: Entries{
		Entry{Hostname: "A"},
		Entry{Hostname: "B"},
		Entry{Hostname: "c"},
		Entry{Hostname: "D"},
		Entry{Hostname: "E"},
		Entry{Hostname: "f"},
	}}

	require.Equal(t, expected, g1)
}
