package net

import (
	"fmt"
	"strings"
)

// External IP addresses
type ExternalIps map[string]bool

func NewExternalIps() ExternalIps {
	return make(map[string]bool)
}
func (ext *ExternalIps) String() string {
	return fmt.Sprint(*ext)
}

// Utility method for setting the external addresses from a list of comma-separated IPs
// Note that we do not remove previously set values.
func (ext ExternalIps) Set(value string) error {
	for _, ipstr := range strings.Split(value, ",") {
		ext[ipstr] = true
	}
	return nil
}
