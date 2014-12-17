package sortinghat

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"github.com/zettio/weave/router"
	wt "github.com/zettio/weave/testing"
	"net"
	"testing"
)

func equal(ms1 Space, ms2 Space) bool {
	return ms1.GetStart().Equal(ms2.GetStart()) &&
		ms1.GetSize() == ms2.GetSize() &&
		ms1.GetMaxAllocated() == ms2.GetMaxAllocated()
}

func (ps1 *PeerSpaceSet) Equal(ps2 *PeerSpaceSet) bool {
	if ps1.version == ps2.version &&
		ps1.peerName == ps2.peerName && ps1.uid == ps2.uid &&
		len(ps1.spaces) == len(ps2.spaces) {
		for i := 0; i < len(ps1.spaces); i++ {
			if !equal(ps1.spaces[i], ps2.spaces[i]) {
				return false
			}
		}
	}
	return true
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
