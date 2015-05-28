package nameserver

import (
	"net"
	"testing"
	"time"

	. "github.com/weaveworks/weave/common"
	wt "github.com/weaveworks/weave/testing"
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

	Debug.Printf("Adding '%s' to Db #1", name)
	dbs[0].Zone.AddRecord("someident", name, net.ParseIP(addr1))

	Debug.Printf("Checking that the name %s is relevant (as it has been locally inserted) and not remote", name)
	wt.AssertTrue(t, dbs[0].Zone.IsNameRelevant(name), "name relevant")
	wt.AssertTrue(t, dbs[0].Zone.HasNameLocalInfo(name), "local name info")
	wt.AssertFalse(t, dbs[0].Zone.HasNameRemoteInfo(name), "remote name info")

	Debug.Printf("Asking for '%s' to Db #1: should get 1 IP from the local database...", name)
	res, err := dbs[0].Zone.DomainLookupName(name)
	wt.AssertNoErr(t, err)
	Debug.Printf("Got: %s", res)
	t.Logf("Db #1 after the lookup:\n%s", dbs[0].Zone)
	wt.AssertEqualInt(t, len(res), 1, "lookup result")

	clk.Forward(refreshInterval / 2)
	Debug.Printf("A couple of seconds later, we should still have one IP for that name")
	res, err = dbs[0].Zone.DomainLookupName(name)
	wt.AssertNoErr(t, err)
	wt.AssertEqualInt(t, len(res), 1, "lookup result")

	Debug.Printf("And then we add 2 IPs for that name at ZoneDb 2")
	clk.Forward(1)
	Debug.Printf("Adding 2 IPs to '%s' in Db #2", name)
	dbs[1].Zone.AddRecord("someident", name, net.ParseIP(addr2))
	dbs[1].Zone.AddRecord("someident", name, net.ParseIP(addr3))

	Debug.Printf("Perform a lookup, to ensure the name will be updated in the background...")
	dbs[0].Zone.DomainLookupName(name)

	Debug.Printf("Wait for a while, until a refresh is performed...")
	clk.Forward(refreshInterval + 1)

	Debug.Printf("A refresh should have been scheduled now: we should have 3 IPs:")
	Debug.Printf("the first (local) IP and the others obtained from zone2 with a mDNS query")
	Debug.Printf("Asking for '%s' again... we should have 3 IPs now", name)
	res, err = dbs[0].Zone.DomainLookupName(name)
	Debug.Printf("Got: %s", res)
	t.Logf("Db #1 after the second lookup:\n%s", dbs[0].Zone)
	wt.AssertEqualInt(t, len(res), 3, "lookup result length")

	Debug.Printf("We will not ask for `name` for a while, so it will become irrelevant and will be removed...")
	clk.Forward(relevantTime + refreshInterval + 1)

	// the name should be irrelevant now, and all remote info should have been
	// removed from the zone database
	Debug.Printf("Name '%s' should not be in the remote database in ZoneDb 1", name)
	wt.AssertFalse(t, dbs[0].Zone.IsNameRelevant(name), "name still relevant after some inactivity time")
	wt.AssertTrue(t, dbs[0].Zone.HasNameLocalInfo(name), "local name info")
	wt.AssertFalse(t, dbs[0].Zone.HasNameRemoteInfo(name), "remote name info")

	Debug.Printf("There is no remote info about this name at zone 1: a new IP appears remotely meanwhile...")
	clk.Forward(1)
	Debug.Printf("Adding '%s' to Db #2", name)
	dbs[1].Zone.AddRecord("someident", name, net.ParseIP(addr4))

	Debug.Printf("When we ask about this name again, we get 4 IPs (1 local, 3 remote)")
	clk.Forward(1)
	Debug.Printf("Asking for '%s' again... the first lookup will return only the local results", name)
	res, err = dbs[0].Zone.DomainLookupName(name)
	wt.AssertEqualInt(t, len(res), 1, "lookup result length")
	Debug.Printf("... but a second lookup should return all the results in the network")
	clk.Forward(1)
	res, err = dbs[0].Zone.DomainLookupName(name)
	Debug.Printf("Got: %s", res)
	wt.AssertEqualInt(t, len(res), 4, "lookup result length") // TODO: this fails 1% of the runs... !!??
}
