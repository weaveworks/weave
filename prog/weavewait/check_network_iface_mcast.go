//go:build iface && mcast
// +build iface,mcast

package main

import (
	weavenet "github.com/rajch/weave/net"
)

func checkNetwork() error {
	_, err := weavenet.EnsureInterfaceAndMcastRoute(weavenet.VethName)
	return err
}
