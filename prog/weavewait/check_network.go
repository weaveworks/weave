// +build !noop

package main

import (
	weavenet "github.com/weaveworks/weave/net"
)

func checkNetwork() error {
	_, err := weavenet.EnsureInterfaceAndMcastRoute("ethwe")
	return err
}
