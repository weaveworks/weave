package nameserver

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	. "github.com/weaveworks/weave/common"
)

func TestAddrs(t *testing.T) {
	InitDefaultLogging(testing.Verbose())
	Info.Println("TestAddrs starting")

	ip, err := addrToIPv4("10.13.12.11")
	require.NoError(t, err)
	require.True(t, net.ParseIP("10.13.12.11").Equal(ip.toNetIP()), "IP")

	ip, err = raddrToIPv4("11.12.13.10.in-addr.arpa.")
	require.NoError(t, err)
	require.True(t, net.ParseIP("10.13.12.11").Equal(ip.toNetIP()), "IP")

	// some malformed addresses
	ip, err = addrToIPv4("10.13.12")
	require.True(t, err != nil, "when parsing malformed address")
	ip, err = addrToIPv4("10.13.AA.12")
	require.True(t, err != nil, "when parsing malformed address")
	ip, err = raddrToIPv4("11.12.13.10.in-axxx.arpa.")
	require.True(t, err != nil, "when parsing malformed address")
	ip, err = raddrToIPv4("11.12.13.10.in-addr")
	require.True(t, err != nil, "when parsing malformed address")
}
