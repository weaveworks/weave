package main

import (
	"fmt"

	weavenet "github.com/weaveworks/weave/net"
)

func uniqueID(args []string) error {
	if len(args) != 1 {
		cmdUsage("unique-id", "<host-root>")
	}
	hostRoot := args[0]
	uid, err := weavenet.GetSystemPeerName(hostRoot)
	if err != nil {
		return err
	}
	fmt.Printf(uid)
	return nil
}
