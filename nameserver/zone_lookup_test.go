package nameserver

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	. "github.com/weaveworks/weave/common"
)

// Check that the refreshing mechanism works as expected
func TestZoneRefresh(t *testing.T) {
	const (
		refreshInterval = 5
		relevantTime    = 30
	)
	var (
		name  = "mysql.weave.local."
		addr1 = "10.2.8.4"
		addr2 = "10.2.8.5"
		addr3 = "10.2.8.6"
		addr4 = "10.2.8.7"
	)

	InitDefaultLogging(testing.Verbose())
	Info.Println("TestZoneRefresh starting")

	clk := newMockedClock()

	// Create multiple zone databases, linked through mocked mDNS connections
	zc := ZoneConfig{
		RefreshInterval: refreshInterval,
		RelevantTime:    relevantTime,
		Clock:           clk,
	}
	dbs := newZoneDbsWithMockedMDns(2, zc)
	dbs.Start()
	defer dbs.Stop()

	time.Sleep(100 * time.Millisecond) // allow for server to get going

	Log.Debugf("Adding '%s' to Db #1", name)
	dbs[0].Zone.AddRecord("someident", name, net.ParseIP(addr1))

	Log.Debugf("Checking that the name %s is relevant (as it has been locally inserted) and not remote", name)
	require.True(t, dbs[0].Zone.IsNameRelevant(name), "name relevant")
	require.True(t, dbs[0].Zone.HasNameLocalInfo(name), "local name info")
	require.False(t, dbs[0].Zone.HasNameRemoteInfo(name), "remote name info")

	Log.Debugf("Asking for '%s' to Db #1: should get 1 IP from the local database...", name)
	res, err := dbs[0].Zone.DomainLookupName(name)
	require.NoError(t, err)
	Log.Debugf("Got: %s", res)
	t.Logf("Db #1 after the lookup:\n%s", dbs[0].Zone)
	require.Equal(t, 1, len(res), "lookup result")

	clk.Forward(refreshInterval / 2)
	dbs.Flush()
	Log.Debugf("A couple of seconds later, we should still have one IP for that name")
	res, err = dbs[0].Zone.DomainLookupName(name)
	require.NoError(t, err)
	require.Equal(t, 1, len(res), "lookup result")

	Log.Debugf("And then we add 2 IPs for that name at ZoneDb 2")
	clk.Forward(1)
	dbs.Flush()
	Log.Debugf("Adding 2 IPs to '%s' in Db #2", name)
	dbs[1].Zone.AddRecord("someident", name, net.ParseIP(addr2))
	dbs[1].Zone.AddRecord("someident", name, net.ParseIP(addr3))

	Log.Debugf("Perform a lookup, to ensure the name will be updated in the background...")
	dbs[0].Zone.DomainLookupName(name)

	Log.Debugf("Wait for a while, until a refresh is performed...")
	clk.Forward(refreshInterval + 1)
	dbs.Flush()
	Log.Debugf("A refresh should have been scheduled now: we should have 3 IPs:")
	Log.Debugf("the first (local) IP and the others obtained from zone2 with a mDNS query")
	Log.Debugf("Asking for '%s' again... we should have 3 IPs now", name)
	res, err = dbs[0].Zone.DomainLookupName(name)
	Log.Debugf("Got: %s", res)
	t.Logf("Db #1 after the second lookup:\n%s", dbs[0].Zone)
	require.Equal(t, 3, len(res), "lookup result length")

	Log.Debugf("We will not ask for `name` for a while, so it will become irrelevant and will be removed...")
	clk.Forward(refreshInterval + relevantTime + 1)
	dbs.Flush()

	// the name should be irrelevant now, and all remote info should have been
	// removed from the zone database
	Log.Debugf("Name '%s' should not be in the remote database in ZoneDb 1", name)
	require.False(t, dbs[0].Zone.IsNameRelevant(name), "name still relevant after some inactivity time")
	require.True(t, dbs[0].Zone.HasNameLocalInfo(name), "local name info")
	require.False(t, dbs[0].Zone.HasNameRemoteInfo(name), "remote name info")

	Log.Debugf("There is no remote info about this name at zone 1: a new IP appears remotely meanwhile...")
	clk.Forward(1)
	dbs.Flush()

	Log.Debugf("Adding '%s' to Db #2", name)
	dbs[1].Zone.AddRecord("someident", name, net.ParseIP(addr4))

	Log.Debugf("When we ask about this name again, we get 4 IPs (1 local, 3 remote)")
	Log.Debugf("Asking for '%s' again... the first lookup will return only the local results", name)
	res, err = dbs[0].Zone.DomainLookupName(name)
	require.Equal(t, 1, len(res), "lookup result length")
	Log.Debugf("... but a second lookup should return all the results in the network")

	clk.Forward(refreshInterval + 1)
	dbs.Flush()

	res, err = dbs[0].Zone.DomainLookupName(name)
	Log.Debugf("Got: %s", res)
	require.Equal(t, 4, len(res), "lookup result length")
}
