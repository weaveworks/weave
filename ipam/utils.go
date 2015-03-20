package ipam

import (
	"bytes"
	"encoding/gob"
	"net"
)

// We shouldn't ever get any errors on *encoding*, but if we do, this will make sure we get to hear about them.
func panicOnError(err error) {
	if err != nil {
		panic(err)
	}
}

// Merge with the one in router/utils.go
func GobEncode(items ...interface{}) []byte {
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	for _, i := range items {
		if spaceSet, ok := i.(SpaceSet); ok {
			panicOnError(spaceSet.Encode(enc))
		} else {
			panicOnError(enc.Encode(i))
		}
	}
	return buf.Bytes()
}

func ip4int(ip4 net.IP) (r uint32) {
	for _, b := range ip4.To4() {
		r <<= 8
		r |= uint32(b)
	}
	return
}

func intip4(key uint32) (r net.IP) {
	r = make([]byte, net.IPv4len)
	for i := 3; i >= 0; i-- {
		r[i] = byte(key)
		key >>= 8
	}
	return
}

// IPv4 Address Arithmetic - convert to 32-bit unsigned integer, add, and convert back
func add(addr net.IP, i uint32) net.IP {
	sum := ip4int(addr) + i
	return intip4(sum)
}

func subtract(a, b net.IP) int64 {
	return int64(ip4int(a)) - int64(ip4int(b))
}
