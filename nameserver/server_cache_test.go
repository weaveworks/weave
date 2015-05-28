package nameserver

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
	. "github.com/weaveworks/weave/common"
	wt "github.com/weaveworks/weave/testing"
)

// Check if the names updates lead to cache invalidations
func TestServerCacheRefresh(t *testing.T) {
	const (
		containerID     = "somecontainer"
		testName1       = "first.weave.local."
		testName2       = "second.weave.local."
		refreshInterval = int(localTTL) / 3
	)

	InitDefaultLogging(testing.Verbose())
	Info.Println("TestServerCacheRefresh starting")
	clk := newMockedClock()

	Debug.Printf("Creating 2 zone databases")
	zoneConfig := ZoneConfig{
		RefreshInterval: refreshInterval,
		Clock:           clk,
	}
	dbs := newZoneDbsWithMockedMDns(2, zoneConfig)
	dbs.Start()
	defer dbs.Stop()

	Debug.Printf("Creating a cache")
	cache, err := NewCache(1024, clk)
	wt.AssertNoErr(t, err)

	Debug.Printf("Creating a real DNS server for the first zone database and with the cache")
	srv, err := NewDNSServer(DNSServerConfig{
		Zone:              dbs[0].Zone,
		Cache:             cache,
		Clock:             clk,
		ListenReadTimeout: testSocketTimeout,
		MaxAnswers:        4,
	})
	wt.AssertNoErr(t, err)
	go srv.Start()
	defer srv.Stop()
	time.Sleep(100 * time.Millisecond) // Allow sever goroutine to start

	testPort, err := srv.GetPort()
	wt.AssertNoErr(t, err)
	wt.AssertNotEqualInt(t, testPort, 0, "listen port")

	Debug.Printf("Adding an IPs to %s", testName1)
	dbs[1].Zone.AddRecord(containerID, testName1, net.ParseIP("10.2.2.1"))

	// Zone database #2 at this point:
	//   first.weave.local  = 10.2.2.1

	// testName1 and testName2 should have no IPs yet
	qName1, _ := assertExchange(t, testName1, dns.TypeA, testPort, 1, 1, 0)
	qName2, _ := assertExchange(t, testName2, dns.TypeA, testPort, 0, 0, dns.RcodeNameError)
	assertInCache(t, cache, qName1, "after asking for first name")
	assertNotLocalInCache(t, cache, qName2, "after asking for second name")

	clk.Forward(refreshInterval / 2)

	Debug.Printf("Adding an IP to %s and to %s", testName1, testName2)
	dbs[1].Zone.AddRecord(containerID, testName1, net.ParseIP("10.2.2.2"))
	dbs[1].Zone.AddRecord(containerID, testName2, net.ParseIP("10.9.9.2"))

	// Zone database #2 at this point:
	//   first.weave.local    = 10.2.2.1 10.2.2.2
	//   second.weave.local   = 10.9.9.2
	clk.Forward(refreshInterval/2 + 2)

	// at this point, the testName1 should have been refreshed
	// so it should have two IPs, and the cache entry should have been invalidated
	assertNotInCache(t, cache, qName1, fmt.Sprintf("after asking for %s", testName1))
	assertNotLocalInCache(t, cache, qName2, fmt.Sprintf("after asking for %s", testName2))

	qName1, _ = assertExchange(t, testName1, dns.TypeA, testPort, 2, 2, 0)
	qName2, _ = assertExchange(t, testName2, dns.TypeA, testPort, 0, 0, dns.RcodeNameError)
	assertInCache(t, cache, qName1, fmt.Sprintf("after asking for %s", testName1))
	assertNotLocalInCache(t, cache, qName2, "after asking for a unknown name")

	// delete the IPs, and some time passes by so the cache should be purged...
	dbs[1].Zone.DeleteRecord(containerID, net.ParseIP("10.2.2.1"))
	dbs[1].Zone.DeleteRecord(containerID, net.ParseIP("10.2.2.2"))
	clk.Forward(refreshInterval + 1)

	qName1, _ = assertExchange(t, testName1, dns.TypeA, testPort, 0, 0, dns.RcodeNameError)
	qName2, _ = assertExchange(t, testName2, dns.TypeA, testPort, 0, 0, dns.RcodeNameError)
	assertNotLocalInCache(t, cache, qName1, "after asking for a unknown name")
	assertNotLocalInCache(t, cache, qName2, "after asking for a unknown name")

}
