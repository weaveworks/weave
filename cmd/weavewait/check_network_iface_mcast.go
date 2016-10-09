// +build iface,mcast

package main

import (
	weavenet "github.com/weaveworks/weave/pkg/net"
)

func checkNetwork() error {
	_, err := weavenet.EnsureInterfaceAndMcastRoute(weavenet.VethName)
	return err
}
