package space

import (
	"github.com/zettio/weave/ipam/utils"
	"net"
	"sort"
)

// For compatibility with Sorting
type addressList []net.IP

func (aa addressList) Len() int           { return len(aa) }
func (aa addressList) Less(i, j int) bool { return utils.IP4int(aa[i]) < utils.IP4int(aa[j]) }
func (aa addressList) Swap(i, j int)      { panic("Should never be swapping entries!") }

// Maintain addresses in increasing order.
func (aa *addressList) add(a net.IP) {
	utils.Assert(sort.IsSorted(*aa), "address list must always be sorted")

	i := sort.Search(len(*aa), func(j int) bool {
		return utils.IP4int((*aa)[j]) >= utils.IP4int(a)
	})

	utils.Assert(i >= len(*aa) || !(*aa)[i].Equal(a), "inserting address into list already exists!")

	(*aa) = append((*aa), nil)   // make space
	copy((*aa)[i+1:], (*aa)[i:]) // move up
	(*aa)[i] = a                 // put in new element

	utils.Assert(sort.IsSorted(aa), "address list must always be sorted")
}

func (aa *addressList) removeAt(pos int) {
	// Delete, preserving order
	(*aa) = append((*aa)[:pos], (*aa)[pos+1:]...)
}

func (aa *addressList) find(addr net.IP) int {
	utils.Assert(sort.IsSorted(*aa), "address list must always be sorted")

	i := sort.Search(len(*aa), func(j int) bool {
		return utils.IP4int((*aa)[j]) >= utils.IP4int(addr)
	})

	if i >= len(*aa) || !(*aa)[i].Equal(addr) {
		// this it not idomatic go; we should set err
		// and return i (as the place said address might go)
		return -1
	}

	return i
}

func (aa *addressList) take() net.IP {
	if n := len(*aa); n <= 0 {
		return nil
	}

	// Always give out the lowest free address
	ret := (*aa)[0]
	*aa = (*aa)[1:]
	return ret
}
