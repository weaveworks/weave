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

func equal(ms1 SpaceInfo, ms2 SpaceInfo) bool {
	return ms1.GetStart().Equal(ms2.GetStart()) &&
		ms1.GetSize() == ms2.GetSize() &&
		ms1.GetMaxAllocated() == ms2.GetMaxAllocated()
}

func (ps1 *PeerSpace) Equal(ps2 *PeerSpace) bool {
	if ps1.PeerName == ps2.PeerName &&
		ps1.version == ps2.version &&
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

	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)

	pn1, _ := router.PeerNameFromString(peer1)
	ps1 := PeerSpace{PeerName: pn1, version: 1234}
	ps1.AddSpace(&MinSpace{Start: net.ParseIP(testAddr1), Size: 10, MaxAllocated: 0})

	err := ps1.Encode(enc)
	wt.AssertNoErr(t, err)

	decoder := gob.NewDecoder(buf)

	var ps2 PeerSpace
	err = ps2.Decode(decoder)
	wt.AssertNoErr(t, err)
	if !ps1.Equal(&ps2) {
		t.Fatalf("Decoded PeerSpace not equal to original")
	}
}
