package ipam

import (
	"bytes"
	"encoding/gob"
	"fmt"
	wt "github.com/zettio/weave/common"
	"github.com/zettio/weave/router"
	"net"
	"testing"
)

func equal(ms1 Space, ms2 Space) bool {
	return ms1.GetStart().Equal(ms2.GetStart()) &&
		ms1.GetSize() == ms2.GetSize() &&
		ms1.GetMaxAllocated() == ms2.GetMaxAllocated()
}

// Note: does not check version
func (ps1 *PeerSpaceSet) Equal(ps2 *PeerSpaceSet) bool {
	if ps1.peerName == ps2.peerName && ps1.uid == ps2.uid &&
		len(ps1.spaces) == len(ps2.spaces) {
		for i := 0; i < len(ps1.spaces); i++ {
			if !equal(ps1.spaces[i], ps2.spaces[i]) {
				return false
			}
		}
		return true
	}
	return false
}

func TestEncodeDecode(t *testing.T) {
	fmt.Println("Starting Encode/Decode test")

	var (
		peer1     = "7a:9f:eb:b6:0c:6e"
		testAddr1 = "10.0.3.4"
	)
	const peer1UID = 123456

	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)

	pn1, _ := router.PeerNameFromString(peer1)
	ps1 := &MutableSpaceSet{PeerSpaceSet{peerName: pn1, uid: peer1UID, version: 1234}}
	ps1.AddSpace(&MutableSpace{MinSpace: MinSpace{Start: net.ParseIP(testAddr1), Size: 10, MaxAllocated: 0}})

	err := ps1.Encode(enc)
	wt.AssertNoErr(t, err)

	decoder := gob.NewDecoder(buf)

	var ps2 PeerSpaceSet
	err = ps2.Decode(decoder)
	wt.AssertNoErr(t, err)
	if ps2.PeerName() != pn1 || ps2.UID() != peer1UID || !ps1.Equal(&ps2) {
		t.Fatalf("Decoded PeerSpace not equal to original")
	}
}

func spaceSetWith(pn router.PeerName, uid uint64, spaces ...*MutableSpace) *MutableSpaceSet {
	ps := NewSpaceSet(pn, uid)
	for _, space := range spaces {
		ps.AddSpace(space)
	}
	return ps
}

func TestExclude(t *testing.T) {
	const (
		peer1     = "7a:9f:eb:b6:0c:6e"
		peer1UID  = 123456
		testAddr1 = "10.0.1.16"
		testAddr2 = "10.0.1.32"
		testAddr3 = "10.0.1.40"
		testAddr4 = "10.0.1.65"
	)

	var (
		ipAddr1 = net.ParseIP(testAddr1)
		ipAddr2 = net.ParseIP(testAddr2)
		ipAddr3 = net.ParseIP(testAddr3)
		ipAddr4 = net.ParseIP(testAddr4)
	)

	pn1, _ := router.PeerNameFromString(peer1)
	// A:  ...--..
	// B:  ..-....
	// E:  ...--..  - no overlap; A stays as-is
	{
		ps1 := spaceSetWith(pn1, peer1UID, NewSpace(ipAddr2, 48))
		ps1.Exclude(NewSpace(ipAddr1, 6))
		ps2 := spaceSetWith(pn1, peer1UID, NewSpace(ipAddr2, 48))
		if !ps1.Equal(&ps2.PeerSpaceSet) {
			t.Fatalf("Exclude failure: expected %s; got %s", ps2, ps1)
		}
	}
	// A:  ..---..
	// B:  ..-....
	// E:  ...--..  - beginning of A is removed
	{
		ps1 := spaceSetWith(pn1, peer1UID, NewSpace(ipAddr1, 48))
		ps1.Exclude(NewSpace(ipAddr1, 16))
		ps2 := spaceSetWith(pn1, peer1UID, NewSpace(ipAddr2, 32))
		if !ps1.Equal(&ps2.PeerSpaceSet) {
			t.Fatalf("Exclude failure: expected %s; got %s", ps2, ps1)
		}
	}
	// A:  ..---..
	// B:  ....-..
	// E:  ..--...  - end of A is removed
	{
		ps1 := spaceSetWith(pn1, peer1UID, NewSpace(ipAddr1, 32))
		ps1.Exclude(NewSpace(ipAddr3, 8))
		ps2 := spaceSetWith(pn1, peer1UID, NewSpace(ipAddr1, 24))
		if !ps1.Equal(&ps2.PeerSpaceSet) {
			t.Fatalf("Exclude failure: expected %s; got %s", ps2, ps1)
		}
	}
	// A:  ..---..
	// B:  ..----.
	// E:  .......  - all of A is removed
	{
		ps1 := spaceSetWith(pn1, peer1UID, NewSpace(ipAddr1, 48))
		ps1.Exclude(NewSpace(ipAddr1, 64))
		ps2 := NewSpaceSet(pn1, peer1UID)
		if !ps1.Equal(&ps2.PeerSpaceSet) {
			t.Fatalf("Exclude failure: expected %s; got %s", ps2, ps1)
		}
	}
	// A:  ..----..
	// B:  ...--...
	// E:  ..-..-..  - A is split into two parts
	{
		ps1 := spaceSetWith(pn1, peer1UID, NewSpace(ipAddr1, 64))
		ps1.Exclude(NewSpace(ipAddr2, 8))
		ps2 := spaceSetWith(pn1, peer1UID, NewSpace(ipAddr1, 16), NewSpace(ipAddr3, 40))
		if !ps1.Equal(&ps2.PeerSpaceSet) {
			t.Fatalf("Exclude failure: expected %s; got %s", ps2, ps1)
		}
	}
	// A:  ..----..--..
	// B:  ....-----...
	// E:  ..--.....-..  - pieces of A are nibbled off
	{
		ps1 := spaceSetWith(pn1, peer1UID, NewSpace(ipAddr1, 20), NewSpace(ipAddr3, 32))
		ps1.Exclude(NewSpace(ipAddr2, 33))
		ps2 := spaceSetWith(pn1, peer1UID, NewSpace(ipAddr1, 16), NewSpace(ipAddr4, 7))
		if !ps1.Equal(&ps2.PeerSpaceSet) {
			t.Fatalf("Exclude failure: expected %s; got %s", ps2, ps1)
		}
	}

}
