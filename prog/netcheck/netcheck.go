/* netcheck: check whether a given network overlaps with any existing routes */
package main

import (
	"fmt"
	"net"
	"os"

	weavenet "github.com/weaveworks/weave/net"
)

func fatal(err error) {
	fmt.Println(err)
	os.Exit(1)
}

func main() {
	if len(os.Args) <= 1 {
		os.Exit(0)
	}

	ipRangeStr := os.Args[1]
	_, ipnet, err := net.ParseCIDR(ipRangeStr)
	if err != nil {
		fatal(err)
	}
	if err := weavenet.CheckNetworkFree(ipnet); err != nil {
		fatal(err)
	}
	os.Exit(0)
}
