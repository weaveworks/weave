/* netcheck: check whether a given network or address overlaps with any existing routes */
package main

import (
	"fmt"
	"net"
	"os"

	"github.com/docker/docker/pkg/mflag"
	weavenet "github.com/weaveworks/weave/net"
)

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

type stringList map[string]struct{}

func (ss *stringList) Set(value string) error {
	(*ss)[value] = struct{}{}
	return nil
}

func (ss *stringList) String() string {
	return fmt.Sprintf("%v", *ss)
}

func main() {
	ignoreIfaceNames := make(stringList)

	mflag.Var(&ignoreIfaceNames, []string{"-ignore-iface"}, "name of interface to ignore)")
	mflag.Parse()

	if len(mflag.Args()) < 1 {
		fmt.Fprintln(os.Stderr, "usage: netcheck [--ignore-iface <iface-name>] network-cidr")
		os.Exit(1)
	}

	cidrStr := mflag.Args()[0]
	addr, ipnet, err := net.ParseCIDR(cidrStr)
	if err != nil {
		fatal(err)
	}
	if ipnet.IP.Equal(addr) {
		err = weavenet.CheckNetworkFree(ipnet, ignoreIfaceNames)
	} else {
		err = weavenet.CheckAddressOverlap(addr, ignoreIfaceNames)
	}
	if err != nil {
		fatal(err)
	}
	os.Exit(0)
}
