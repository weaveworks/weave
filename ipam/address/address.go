package address

import (
	"github.com/weaveworks/weave/common"
	"net"
)

// Using 32-bit integer to represent IPv4 address
type Address uint32
type Offset uint32

type Range struct {
	Start, End Address // [Start, End); Start <= End
}

func ParseIP(s string) (Address, error) {
	if ip := net.ParseIP(s); ip == nil {
		return 0, &net.ParseError{Type: "IP Address", Text: s}
	} else {
		return FromIP4(ip), nil
	}
}

// FromIP4 converts an ipv4 address to our integer address type
func FromIP4(ip4 net.IP) (r Address) {
	for _, b := range ip4.To4() {
		r <<= 8
		r |= Address(b)
	}
	return
}

// IP4 converts our integer address type to an ipv4 address
func (addr Address) IP4() (r net.IP) {
	r = make([]byte, net.IPv4len)
	for i := 3; i >= 0; i-- {
		r[i] = byte(addr)
		addr >>= 8
	}
	return
}

func (addr Address) String() string {
	return addr.IP4().String()
}

func Add(addr Address, i Offset) Address {
	return addr + Address(i)
}

func Subtract(a, b Address) Offset {
	common.Assert(a >= b)
	return Offset(a - b)
}
