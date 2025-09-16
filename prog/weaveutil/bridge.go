package main

import (
	"fmt"

	weavenet "github.com/rajch/weave/net"
)

func detectBridgeType(args []string) error {
	if len(args) != 2 {
		cmdUsage("detect-bridge-type", "<weave-bridge-name> <datapath-name>")
	}
	bridgeType, err := weavenet.ExistingBridgeType(args[0], args[1])
	if err != nil {
		return err
	} else if bridgeType == nil {
		fmt.Println("none")
	} else {
		fmt.Println(bridgeType.String())
	}
	return nil
}
