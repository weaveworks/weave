package nameserver

import (
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
	. "github.com/weaveworks/weave/common"
)

// Check that we can use a regular mDNS server with a regular mDNS client
func TestClientServerSimpleQuery(t *testing.T) {
	InitDefaultLogging(testing.Verbose())

	testRecord1 := Record{"test.weave.local.", net.ParseIP("10.2.2.1"), 0, 0, 0}
	testInAddr1 := "1.2.2.10.in-addr.arpa."

	mzone := newMockedZoneWithRecords([]ZoneRecord{testRecord1})
	mdnsServer, err := NewMDNSServer(mzone, true, DefaultLocalTTL)
	require.NoError(t, err)
	err = mdnsServer.Start(nil)
	require.NoError(t, err)
	defer mdnsServer.Stop()

	var receivedAddr net.IP
	var receivedName string
	receivedCount := 0

	mdnsCli, err := NewMDNSClient()
	require.NoError(t, err)
	err = mdnsCli.Start(nil)
	require.NoError(t, err)

	sendQuery := func(name string, querytype uint16) {
		receivedAddr = nil
		receivedName = ""
		receivedCount = 0
		Log.Debugf("Sending query...")
		switch querytype {
		case dns.TypeA:
			r, err := mdnsCli.LookupName(name)
			if len(r) > 0 {
				receivedAddr = r[0].IP()
				if err == nil {
					receivedCount++
				}
			}
		case dns.TypePTR:
			r, err := mdnsCli.LookupInaddr(name)
			if len(r) > 0 {
				receivedName = r[0].Name()
				if err == nil {
					receivedCount++
				}
			}
		}
	}

	time.Sleep(100 * time.Millisecond) // Allow for server to get going

	Log.Debugf("Query: %s dns.TypeA", testRecord1.Name())
	sendQuery(testRecord1.Name(), dns.TypeA)
	if receivedCount != 1 {
		t.Fatalf("Unexpected result count %d for %s", receivedCount, testRecord1.Name())
	}
	if !receivedAddr.Equal(testRecord1.IP()) {
		t.Fatalf("Unexpected result %s for %s", receivedAddr, testRecord1.Name())
	}

	Log.Debugf("Query: testfail.weave. dns.TypeA")
	sendQuery("testfail.weave.", dns.TypeA)
	if receivedCount != 0 {
		t.Fatalf("Unexpected result count %d for testfail.weave", receivedCount)
	}

	Log.Debugf("Query: %s dns.TypePTR", testInAddr1)
	sendQuery(testInAddr1, dns.TypePTR)
	if receivedCount != 1 {
		t.Fatalf("Expected an answer to %s, got %d answers", testInAddr1, receivedCount)
	} else if !(testRecord1.Name() == receivedName) {
		t.Fatalf("Expected answer %s to query for %s, got %s", testRecord1.Name(), testInAddr1, receivedName)
	}
}

// Check that we can use a use "insistent" queries
func TestClientServerInsistentQuery(t *testing.T) {
	InitDefaultLogging(testing.Verbose())

	testRecord1 := Record{"test.weave.local.", net.ParseIP("10.2.2.1"), 0, 0, 0}
	testInAddr1 := "1.2.2.10.in-addr.arpa."
	testRecord2 := Record{"test.weave.local.", net.ParseIP("10.2.2.2"), 0, 0, 0}
	testInAddr2 := "2.2.2.10.in-addr.arpa."

	mzone1 := newMockedZoneWithRecords([]ZoneRecord{testRecord1})
	mdnsServer1, err := NewMDNSServer(mzone1, true, DefaultLocalTTL)
	require.NoError(t, err)
	err = mdnsServer1.Start(nil)
	require.NoError(t, err)
	defer mdnsServer1.Stop()

	mzone2 := newMockedZoneWithRecords([]ZoneRecord{testRecord2})
	mdnsServer2, err := NewMDNSServer(mzone2, true, DefaultLocalTTL)
	require.NoError(t, err)
	err = mdnsServer2.Start(nil)
	require.NoError(t, err)
	defer mdnsServer2.Stop()

	// create a third server with exactly the same info as the second server (so we can test duplicates removals)
	mdnsServer3, err := NewMDNSServer(mzone2, true, DefaultLocalTTL)
	require.NoError(t, err)
	err = mdnsServer3.Start(nil)
	require.NoError(t, err)
	defer mdnsServer3.Stop()

	var receivedAddrs []ZoneRecord
	var receivedNames []ZoneRecord
	receivedCount := 0

	mdnsCli, err := NewMDNSClient()
	require.NoError(t, err)
	err = mdnsCli.Start(nil)
	require.NoError(t, err)

	sendQuery := func(name string, querytype uint16) {
		receivedAddrs = nil
		receivedNames = nil
		receivedCount = 0
		Log.Debugf("Sending query...")
		switch querytype {
		case dns.TypeA:
			receivedAddrs, err = mdnsCli.InsistentLookupName(name)
			if err == nil {
				receivedCount = len(receivedAddrs)
			}
		case dns.TypePTR:
			receivedNames, err = mdnsCli.InsistentLookupInaddr(name)
			if err == nil {
				receivedCount = len(receivedNames)
			}
		}
	}

	time.Sleep(100 * time.Millisecond) // Allow for server to get going

	Log.Debugf("Query: %s dns.TypeA", testRecord1.Name())
	sendQuery(testRecord1.Name(), dns.TypeA)
	if receivedCount != 2 {
		t.Fatalf("Unexpected result count %d for %s", receivedCount, testRecord1.Name())
	}

	Log.Debugf("Query: testfail.weave. dns.TypeA")
	sendQuery("testfail.weave.", dns.TypeA)
	if receivedCount != 0 {
		t.Fatalf("Unexpected result count %d for testfail.weave", receivedCount)
	}

	Log.Debugf("Query: %s dns.TypePTR", testInAddr1)
	sendQuery(testInAddr1, dns.TypePTR)
	if receivedCount != 1 {
		t.Fatalf("Expected an answer to %s, got %d answers", testInAddr1, receivedCount)
	}

	Log.Debugf("Query: %s dns.TypePTR", testInAddr2)
	sendQuery(testInAddr2, dns.TypePTR)
	if receivedCount != 1 {
		t.Fatalf("Expected an answer to %s, got %d answers", testInAddr2, receivedCount)
	}
}
