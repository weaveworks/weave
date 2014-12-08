package net

import (
	"fmt"
	"net"
	"strings"
)

// external IP addresses
type ExternalIps []net.IP

func (i *ExternalIps) String() string {
	return fmt.Sprint(*i)
}

// Utility methd for setting the external addresses from a list of comma-separated IPs
func (i *ExternalIps) Set(value string) error {
	for _, ipstr := range strings.Split(value, ",") {
		ip := net.ParseIP(ipstr)
		*i = append(*i, ip)
	}
	return nil
}
