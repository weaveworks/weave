package nameserver

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
	. "github.com/weaveworks/weave/common"
)

// Check that AddRecord/DeleteRecord/... in the Zone database lead to cache invalidations
func TestServerDbCacheInvalidation(t *testing.T) {
	const (
		containerID = "somecontainer"
		testName1   = "first.weave.local."
		testName2   = "second.weave.local."
	)

	InitDefaultLogging(testing.Verbose())
	Info.Println("TestServerDbCacheInvalidation starting")

	clk := newMockedClock()

	Log.Debugf("Creating mocked mDNS client and server")
	mdnsServer1 := newMockedMDNSServerWithRecord(Record{testName1, net.ParseIP("10.2.2.9"), 0, 0, 0})
	mdnsCli1 := newMockedMDNSClient([]*mockedMDNSServer{mdnsServer1})

	Log.Debugf("Creating zone database with the mocked mDNS client and server")
	zoneConfig := ZoneConfig{
		MDNSServer: mdnsServer1,
		MDNSClient: mdnsCli1,
		Clock:      clk,
	}
	zone, err := NewZoneDb(zoneConfig)
	require.NoError(t, err)
	err = zone.Start()
	require.NoError(t, err)
	defer zone.Stop()

	Log.Debugf("Creating a cache")
	cache, err := NewCache(1024, clk)
	require.NoError(t, err)

	fallbackHandler := func(w dns.ResponseWriter, req *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(req)
		if len(req.Question) == 1 {
			m.Rcode = dns.RcodeNameError
		}
		w.WriteMsg(m)
	}

	// Run another DNS server for fallback
	fallback, err := newMockedFallback(fallbackHandler, nil)
	require.NoError(t, err)
	fallback.Start()
	defer fallback.Stop()

	Log.Debugf("Creating a real DNS server with a mocked cache")
	srv, err := NewDNSServer(DNSServerConfig{
		Zone:              zone,
		Cache:             cache,
		Clock:             clk,
		ListenReadTimeout: testSocketTimeout,
		UpstreamCfg:       fallback.CliConfig,
		MaxAnswers:        4,
	})
	require.NoError(t, err)
	err = srv.Start()
	require.NoError(t, err)
	go srv.ActivateAndServe()
	defer srv.Stop()
	time.Sleep(100 * time.Millisecond) // Allow server goroutine to start

	testPort, err := srv.GetPort()
	require.NoError(t, err)
	require.NotEqual(t, 0, testPort, "invalid listen port")

	Log.Debugf("Adding two IPs to %s", testName1)
	zone.AddRecord(containerID, testName1, net.ParseIP("10.2.2.1"))
	zone.AddRecord(containerID, testName1, net.ParseIP("10.2.2.2"))
	q, _ := assertExchange(t, testName1, dns.TypeA, testPort, 2, 2, 0)
	assertInCache(t, cache, q, fmt.Sprintf("after asking for %s", testName1))

	// Zone database at this point:
	//   first.weave.local  = 10.2.2.1 10.2.2.2

	zone.AddRecord(containerID, testName2, net.ParseIP("10.9.9.1"))
	assertInCache(t, cache, q, fmt.Sprintf("after adding a new IP for %s", testName2))

	// we should have an entry in the cache for this query
	// if we add another IP, that cache entry should be removed
	Log.Debugf("Adding a new IP to %s: the cache entry should be removed", testName1)
	zone.AddRecord(containerID, testName1, net.ParseIP("10.2.2.3"))
	assertNotInCache(t, cache, q, fmt.Sprintf("after adding a new IP for %s", testName1))

	// Zone database at this point:
	//   first.weave.local  = 10.2.2.1 10.2.2.2 10.2.2.3
	//   second.weave.local = 10.9.9.1

	Log.Debugf("Querying again (so a cache entry will be created)")
	q, _ = assertExchange(t, testName1, dns.TypeA, testPort, 3, 4, 0)
	assertInCache(t, cache, q, "after asking about the name")
	Log.Debugf("... and removing one of the IP addresses")
	zone.DeleteRecords(containerID, "", net.ParseIP("10.2.2.2"))
	assertNotInCache(t, cache, q, "after deleting IP for 10.2.2.2")

	// Zone database at this point:
	//   first.weave.local  = 10.2.2.1 10.2.2.3
	//   second.weave.local = 10.9.9.1

	// generate cache responses
	Log.Debugf("Querying for a raddr")
	qname, _ := assertExchange(t, testName1, dns.TypeA, testPort, 2, 2, 0)
	qptr, _ := assertExchange(t, "1.2.2.10.in-addr.arpa.", dns.TypePTR, testPort, 1, 1, 0)
	qotherName, _ := assertExchange(t, testName2, dns.TypeA, testPort, 1, 1, 0)
	qotherPtr, _ := assertExchange(t, "1.9.9.10.in-addr.arpa.", dns.TypePTR, testPort, 1, 1, 0)
	qwrongName, _ := assertExchange(t, "wrong.weave.local.", dns.TypeA, testPort, 0, 0, dns.RcodeNameError)
	assertInCache(t, cache, qname, "after asking for name")
	assertInCache(t, cache, qptr, "after asking for address")
	assertInCache(t, cache, qotherName, "after asking for second name")
	assertInCache(t, cache, qotherPtr, "after asking for second address")
	assertNotLocalInCache(t, cache, qwrongName, "after asking for a wrong name")

	// now we will check if a removal affects all the responses
	Log.Debugf("... and removing an IP should invalidate both the cached responses for name and raddr")
	zone.DeleteRecords(containerID, "", net.ParseIP("10.2.2.1"))
	assertNotInCache(t, cache, qptr, "after deleting record")
	assertNotInCache(t, cache, qname, "after deleting record")
	assertInCache(t, cache, qotherName, "after deleting record")

	// Zone database at this point:
	//   first.weave.local  = 10.2.2.3
	//   second.weave.local = 10.9.9.1

	// generate cache responses
	Log.Debugf("Querying for a raddr")
	qptr, _ = assertExchange(t, "3.2.2.10.in-addr.arpa.", dns.TypePTR, testPort, 1, 1, 0)
	qname, _ = assertExchange(t, testName1, dns.TypeA, testPort, 1, 1, 0)
	qotherName, _ = assertExchange(t, testName2, dns.TypeA, testPort, 1, 1, 0)
	qotherPtr, _ = assertExchange(t, "1.9.9.10.in-addr.arpa.", dns.TypePTR, testPort, 1, 1, 0)
	assertInCache(t, cache, qname, "after asking for name")
	assertInCache(t, cache, qptr, "after asking for PTR")
	assertInCache(t, cache, qotherName, "after asking for second name")
	assertInCache(t, cache, qotherPtr, "after asking for second address")

	// let's repeat this, but adding an IP
	Log.Debugf("... and adding a new IP should invalidate both the cached responses for the name")
	zone.AddRecord(containerID, testName1, net.ParseIP("10.2.2.7"))
	assertNotInCache(t, cache, qname, "after adding a new IP")
	assertInCache(t, cache, qotherName, "after adding a new IP")
	assertInCache(t, cache, qotherPtr, "after adding a new IP")

	// check that after some time, the cache entry is expired
	clk.Forward(int(DefaultLocalTTL) + 1)
	assertNotInCache(t, cache, qotherName, "after passing some time")
	assertNotInCache(t, cache, qwrongName, "after passing some time")

	// Zone database at this point:
	//   first.weave.local  = 10.2.2.3 10.2.2.7
	//   second.weave.local = 10.9.9.1

	zone.DeleteRecords(containerID, "", net.IP{})
	assertNotInCache(t, cache, qotherName, "after removing container")
	assertNotInCache(t, cache, qotherPtr, "after removing container")
}

// Check if the entries in the cache expire
func TestServerCacheExpiration(t *testing.T) {
	const (
		containerID      = "somecontainer"
		testName1        = "first.weave.local."
		testName2        = "second.weave.local."
		negativeLocalTTL = 10
	)

	InitDefaultLogging(testing.Verbose())
	Info.Println("TestServerCacheExpiration starting")

	clk := newMockedClock()

	Log.Debugf("Creating 2 zone databases")
	zoneConfig := ZoneConfig{
		RefreshInterval: 0, // no name updates
		Clock:           clk,
	}
	dbs := newZoneDbsWithMockedMDns(2, zoneConfig)
	dbs.Start()
	defer dbs.Stop()

	Log.Debugf("Creating a cache")
	cache, err := NewCache(1024, clk)
	require.NoError(t, err)

	Log.Debugf("Creating a real DNS server for the first zone database and with the cache")
	srv, err := NewDNSServer(DNSServerConfig{
		Zone:              dbs[0].Zone,
		Cache:             cache,
		Clock:             clk,
		ListenReadTimeout: testSocketTimeout,
		CacheNegLocalTTL:  negativeLocalTTL,
	})
	require.NoError(t, err)
	err = srv.Start()
	require.NoError(t, err)
	go srv.ActivateAndServe()
	defer srv.Stop()
	time.Sleep(100 * time.Millisecond) // Allow server goroutine to start

	testPort, err := srv.GetPort()
	require.NoError(t, err)
	require.NotEqual(t, 0, testPort, "invalid listen port")

	// Check that the DNS server knows nothing about testName1
	qName1, _ := assertExchange(t, testName1, dns.TypeA, testPort, 0, 0, dns.RcodeNameError)
	// there should be a negative reply cached during [0, negativeLocalTTL]
	assertNotLocalInCache(t, cache, qName1, fmt.Sprintf("after asking for %s", testName1))
	clk.Forward(negativeLocalTTL - 1)
	assertNotLocalInCache(t, cache, qName1, fmt.Sprintf("after asking for %s", testName1))
	// after negativeLocalTTL, there should be absolutely nothing in the cache
	clk.Forward(2)
	assertNotInCache(t, cache, qName1, fmt.Sprintf("after asking for %s", testName1))

	// We add the IP in the second peer right after
	Log.Debugf("Adding an IPs to %s in database #2", testName1)
	dbs[1].Zone.AddRecord(containerID, testName1, net.ParseIP("10.2.2.1"))
	t.Logf("Zone database #2:\n%s", dbs[1].Zone)

	// Check that the DNS server returns updated information, and it will be
	// cached during [0, DefaultLocalTTL]
	qName1, _ = assertExchange(t, testName1, dns.TypeA, testPort, 1, 1, 0)

	assertInCache(t, cache, qName1, fmt.Sprintf("after asking for %s", testName1))
	clk.Forward(int(DefaultLocalTTL) - 1)
	assertInCache(t, cache, qName1, fmt.Sprintf("after asking for %s", testName1))
	// and it will expire after DefaultLocalTTL
	clk.Forward(2)
	assertNotInCache(t, cache, qName1, fmt.Sprintf("after asking for %s", testName1))

	// We delete the IP in the second peer right after
	Log.Debugf("Removing the IPs from %s in database #2", testName1)
	dbs[1].Zone.DeleteRecords(containerID, "", net.ParseIP("10.2.2.1"))

	// Check that we return to the initial state: a neg-local entry in the cache
	qName1, _ = assertExchange(t, testName1, dns.TypeA, testPort, 0, 0, dns.RcodeNameError)
	assertNotLocalInCache(t, cache, qName1, fmt.Sprintf("after asking for %s", testName1))
	clk.Forward(negativeLocalTTL + 1)

	// Check that all entries should be outdated
	cache.Purge()
	if cache.Len() != 0 {
		t.Fatalf("Some entries still valid in cache:\n%s", cache)
	}
}

// Check if the names updates lead to cache invalidations
func TestServerCacheRefresh(t *testing.T) {
	const (
		containerID     = "somecontainer"
		testName1       = "first.weave.local."
		testName2       = "second.weave.local."
		refreshInterval = int(DefaultLocalTTL) / 3
	)

	InitDefaultLogging(testing.Verbose())
	Info.Println("TestServerCacheRefresh starting")
	clk := newMockedClock()

	Log.Debugf("Creating 2 zone databases")
	zoneConfig := ZoneConfig{
		RefreshInterval: refreshInterval,
		Clock:           clk,
	}
	dbs := newZoneDbsWithMockedMDns(2, zoneConfig)
	dbs.Start()
	defer dbs.Stop()

	Log.Debugf("Creating a cache")
	cache, err := NewCache(1024, clk)
	require.NoError(t, err)

	Log.Debugf("Creating a real DNS server for the first zone database and with the cache")
	srv, err := NewDNSServer(DNSServerConfig{
		Zone:              dbs[0].Zone,
		Cache:             cache,
		Clock:             clk,
		ListenReadTimeout: testSocketTimeout,
		MaxAnswers:        4,
	})
	require.NoError(t, err)
	err = srv.Start()
	require.NoError(t, err)
	go srv.ActivateAndServe()
	defer srv.Stop()
	time.Sleep(100 * time.Millisecond) // Allow sever goroutine to start

	testPort, err := srv.GetPort()
	require.NoError(t, err)
	require.NotEqual(t, 0, testPort, "listen port")

	Log.Debugf("Adding an IPs to %s", testName1)
	dbs[1].Zone.AddRecord(containerID, testName1, net.ParseIP("10.2.2.1"))
	dbs.Flush() // ensure the immediate refresh is done

	// Zone database #2 at this point:
	//   first.weave.local  = 10.2.2.1

	// testName1 and testName2 should have no IPs yet
	qName1, _ := assertExchange(t, testName1, dns.TypeA, testPort, 1, 1, 0)
	qName2, _ := assertExchange(t, testName2, dns.TypeA, testPort, 0, 0, dns.RcodeNameError)
	assertInCache(t, cache, qName1, "after asking for first name")
	assertNotLocalInCache(t, cache, qName2, "after asking for second name")

	clk.Forward(refreshInterval / 2)
	dbs.Flush()

	Log.Debugf("Adding an IP to %s and to %s", testName1, testName2)
	dbs[1].Zone.AddRecord(containerID, testName1, net.ParseIP("10.2.2.2"))
	dbs[1].Zone.AddRecord(containerID, testName2, net.ParseIP("10.9.9.2"))

	// Zone database #2 at this point:
	//   first.weave.local    = 10.2.2.1 10.2.2.2
	//   second.weave.local   = 10.9.9.2
	clk.Forward(refreshInterval/2 + 2)
	dbs.Flush()

	// at this point, the testName1 should have been refreshed
	// so it should have two IPs, and the cache entry should have been invalidated
	assertNotInCache(t, cache, qName1, fmt.Sprintf("after asking for %s", testName1))
	assertNotLocalInCache(t, cache, qName2, fmt.Sprintf("after asking for %s", testName2))

	qName1, _ = assertExchange(t, testName1, dns.TypeA, testPort, 2, 2, 0)
	qName2, _ = assertExchange(t, testName2, dns.TypeA, testPort, 0, 0, dns.RcodeNameError)
	assertInCache(t, cache, qName1, fmt.Sprintf("after asking for %s", testName1))
	assertNotLocalInCache(t, cache, qName2, "after asking for a unknown name")

	// delete the IPs, and some time passes by so the cache should be purged...
	dbs[1].Zone.DeleteRecords(containerID, "", net.ParseIP("10.2.2.1"))
	dbs[1].Zone.DeleteRecords(containerID, "", net.ParseIP("10.2.2.2"))
	clk.Forward(refreshInterval + 1)
	dbs.Flush()

	qName1, _ = assertExchange(t, testName1, dns.TypeA, testPort, 0, 0, dns.RcodeNameError)
	qName2, _ = assertExchange(t, testName2, dns.TypeA, testPort, 0, 0, dns.RcodeNameError)
	assertNotLocalInCache(t, cache, qName1, "after asking for a unknown name")
	assertNotLocalInCache(t, cache, qName2, "after asking for a unknown name")

}
