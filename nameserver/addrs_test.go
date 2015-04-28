package nameserver

import (
	. "github.com/weaveworks/weave/common"
	wt "github.com/weaveworks/weave/testing"
	"net"
	"testing"
)

func TestAddrs(t *testing.T) {
	InitDefaultLogging(testing.Verbose())
	Info.Println("TestAddrs starting")

	ip, err := addrToIPv4("10.13.12.11")
	wt.AssertNoErr(t, err)
	wt.AssertTrue(t, net.ParseIP("10.13.12.11").Equal(ip.toNetIP()), "IP")

	ip, err = raddrToIPv4("11.12.13.10.in-addr.arpa.")
	wt.AssertNoErr(t, err)
	wt.AssertTrue(t, net.ParseIP("10.13.12.11").Equal(ip.toNetIP()), "IP")

	// some malformed addresses
	ip, err = addrToIPv4("10.13.12")
	wt.AssertTrue(t, err != nil, "when parsing malformed address")
	ip, err = addrToIPv4("10.13.AA.12")
	wt.AssertTrue(t, err != nil, "when parsing malformed address")
	ip, err = raddrToIPv4("11.12.13.10.in-axxx.arpa.")
	wt.AssertTrue(t, err != nil, "when parsing malformed address")
	ip, err = raddrToIPv4("11.12.13.10.in-addr")
	wt.AssertTrue(t, err != nil, "when parsing malformed address")
}
