// Copyright 2021 CNI authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ip

import (
	"fmt"
	"net"
	"strings"
)

// IP is a CNI maintained type inherited from net.IPNet which can
// represent a single IP address with or without prefix.
type IP struct {
	net.IPNet
}

// newIP will create an IP with net.IP and net.IPMask
func newIP(ip net.IP, mask net.IPMask) *IP {
	return &IP{
		IPNet: net.IPNet{
			IP:   ip,
			Mask: mask,
		},
	}
}

// ParseIP will parse string s as an IP, and return it.
// The string s must be formed like <ip>[/<prefix>].
// If s is not a valid textual representation of an IP,
// will return nil.
func ParseIP(s string) *IP {
	if strings.ContainsAny(s, "/") {
		ip, ipNet, err := net.ParseCIDR(s)
		if err != nil {
			return nil
		}
		return newIP(ip, ipNet.Mask)
	} else {
		ip := net.ParseIP(s)
		if ip == nil {
			return nil
		}
		return newIP(ip, nil)
	}
}

// ToIP will return a net.IP in standard form from this IP.
// If this IP can not be converted to a valid net.IP, will return nil.
func (i *IP) ToIP() net.IP {
	switch {
	case i.IP.To4() != nil:
		return i.IP.To4()
	case i.IP.To16() != nil:
		return i.IP.To16()
	default:
		return nil
	}
}

// String returns the string form of this IP.
func (i *IP) String() string {
	if len(i.Mask) > 0 {
		return i.IPNet.String()
	}
	return i.IP.String()
}

// MarshalText implements the encoding.TextMarshaler interface.
// The encoding is the same as returned by String,
// But when len(ip) is zero, will return an empty slice.
func (i *IP) MarshalText() ([]byte, error) {
	if len(i.IP) == 0 {
		return []byte{}, nil
	}
	return []byte(i.String()), nil
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
// The textual bytes are expected in a form accepted by Parse,
// But when len(b) is zero, will return an empty IP.
func (i *IP) UnmarshalText(b []byte) error {
	if len(b) == 0 {
		*i = IP{}
		return nil
	}

	ip := ParseIP(string(b))
	if ip == nil {
		return fmt.Errorf("invalid IP address %s", string(b))
	}
	*i = *ip
	return nil
}
