/* netcheck: check whether a given network or address overlaps with any existing routes */
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

	cidrStr := os.Args[1]
	addr, ipnet, err := net.ParseCIDR(cidrStr)
	if err != nil {
		fatal(err)
	}
	if ipnet.IP.Equal(addr) {
		err = weavenet.CheckNetworkFree(ipnet)
	} else {
		err = weavenet.CheckAddressOverlap(addr)
	}
	if err != nil {
		fatal(err)
	}
	os.Exit(0)
}
