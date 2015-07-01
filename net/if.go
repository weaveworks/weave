package net

import (
	"fmt"
	"math"
	"net"
	"time"
)

const (
	sleepTime = 100 * time.Millisecond
)

// Wait `wait` seconds for an interface to come up. Pass zero to check once
// and return immediately, or a negative value to wait indefinitely.
func EnsureInterface(ifaceName string, wait int) (iface *net.Interface, err error) {
	if iface, err = findInterface(ifaceName); err == nil || wait == 0 {
		return
	}
	i := int64(math.MaxInt64)
	if wait > 0 {
		i = int64(time.Duration(wait) * time.Second / sleepTime)
	}
	for ; err != nil && i > 0; i-- {
		time.Sleep(sleepTime)
		iface, err = findInterface(ifaceName)
	}
	return
}

func findInterface(ifaceName string) (iface *net.Interface, err error) {
	if iface, err = net.InterfaceByName(ifaceName); err != nil {
		return iface, fmt.Errorf("Unable to find interface %s", ifaceName)
	}
	if 0 == (net.FlagUp & iface.Flags) {
		return iface, fmt.Errorf("Interface %s is not up", ifaceName)
	}
	return
}
