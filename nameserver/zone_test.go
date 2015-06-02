package nameserver

import (
	"fmt"
	"net"
	"testing"

	. "github.com/weaveworks/weave/common"
	wt "github.com/weaveworks/weave/testing"
)

func TestZone(t *testing.T) {
	var (
		container1    = "deadbeef"
		container2    = "cowjuice"
		name1         = "test1.weave."
		name1Addr1    = "10.9.2.1/24"
		name1Addr2    = "10.9.2.2/24"
		revName1Addr1 = "1.2.9.10.in-addr.arpa."
	)

	InitDefaultLogging(testing.Verbose())

	zone, err := NewZoneDb(ZoneConfig{})
	wt.AssertNoErr(t, err)
	err = zone.Start()
	wt.AssertNoErr(t, err)
	defer zone.Stop()

	ip1, _, _ := net.ParseCIDR(name1Addr1)
	t.Logf("Adding '%s'/%s to '%s'", name1, ip1, container1)
	err = zone.AddRecord(container1, name1, ip1)
	wt.AssertNoErr(t, err)

	// Add a few more records to make the job harder
	t.Logf("Adding 'adummy.weave.' to 'abcdef0123'")
	err = zone.AddRecord("abcdef0123", "adummy.weave.", net.ParseIP("10.9.0.1"))
	wt.AssertNoErr(t, err)
	t.Logf("Adding 'zdummy.weave.' to '0123abcdef'")
	err = zone.AddRecord("0123abcdef", "zdummy.weave.", net.ParseIP("10.9.0.2"))
	wt.AssertNoErr(t, err)
	t.Logf("Zone database:\n%s", zone)

	t.Logf("Checking if we can find the name '%s'", name1)
	foundIPs, err := zone.LookupName(name1)
	wt.AssertNoErr(t, err)

	if !foundIPs[0].IP().Equal(ip1) {
		t.Fatal("Unexpected result for", name1, foundIPs)
	}

	t.Logf("Checking if we cannot find some silly name like 'something.wrong'")
	foundIPs, err = zone.LookupName("something.wrong")
	wt.AssertErrorType(t, err, (*LookupError)(nil), fmt.Sprintf("unknown name: %+v", foundIPs))

	ip2, _, _ := net.ParseCIDR(name1Addr2)
	t.Logf("Adding a second IP for '%s'/%s to '%s'", name1, ip2, container1)
	err = zone.AddRecord(container1, name1, ip2)
	wt.AssertNoErr(t, err)

	t.Logf("Checking if we can find both the old IP and the new IP for '%s'", name1)
	foundIPs, err = zone.LookupName(name1)
	wt.AssertNoErr(t, err)
	if !(foundIPs[0].IP().Equal(ip1) || foundIPs[1].IP().Equal(ip1)) {
		t.Fatal("Unexpected result for", name1, foundIPs)
	}
	if !(foundIPs[0].IP().Equal(ip2) || foundIPs[1].IP().Equal(ip2)) {
		t.Fatal("Unexpected result for", name1, foundIPs)
	}

	t.Logf("Checking if we can find the address by IP '1.2.9.10.in-addr.arpa.'")
	foundNames, err := zone.LookupInaddr("1.2.9.10.in-addr.arpa.")
	wt.AssertNoErr(t, err)

	if foundNames[0].Name() != name1 {
		t.Fatal("Unexpected result for", ip1, foundNames)
	}

	t.Logf("Checking we can not find an unknown address '30.20.10.1.in-addr.arpa.'")
	foundNames, err = zone.LookupInaddr("30.20.10.1.in-addr.arpa.")
	wt.AssertErrorType(t, err, (*LookupError)(nil), fmt.Sprintf("unknown IP: %+v", foundNames))

	t.Logf("Checking if adding again '%s'/%s in %s results in an error", name1, ip1, container1)
	err = zone.AddRecord(container1, name1, ip1)
	wt.AssertErrorType(t, err, (*DuplicateError)(nil), "duplicate add")

	t.Logf("Adding '%s'/%s in %s too", name1, ip1, container2)
	err = zone.AddRecord(container2, name1, ip1)
	wt.AssertNoErr(t, err)

	name1Removed := 0
	err = zone.ObserveName(name1, func() { t.Logf("Observer #1 for '%s' notified.", name1); name1Removed++ })
	wt.AssertNoErr(t, err)
	err = zone.ObserveName(name1, func() { t.Logf("Observer #2 for '%s' notified.", name1); name1Removed++ })
	wt.AssertNoErr(t, err)
	err = zone.ObserveInaddr(revName1Addr1, func() { t.Logf("Observer #1 for '%s' notified.", revName1Addr1); name1Removed++ })
	wt.AssertNoErr(t, err)

	t.Logf("Zone database:\n%s", zone)
	t.Logf("Deleting the %s in %s", ip1, container1)
	count := zone.DeleteRecords(container1, "", ip1)
	wt.AssertEqualInt(t, count, 1, "delete failed")
	t.Logf("Zone database:\n%s", zone)

	t.Logf("Checking %s's observers have been notified on removal", name1)
	if name1Removed < 3 {
		t.Logf("Zone database:\n%s", zone)
		t.Fatalf("Unexpected number (%d) of calls to observers", name1Removed)
	}

	t.Logf("Checking %s can be found", name1)
	_, err = zone.LookupName(name1)
	wt.AssertNoErr(t, err)

	t.Logf("Checking %s is not found after removing %s it in %s and %s in %s",
		name1, ip1, container2, ip2, container1)
	count = zone.DeleteRecords(container1, "", ip2)
	wt.AssertEqualInt(t, count, 1, "delete failed")
	count = zone.DeleteRecords(container2, "", ip1)
	wt.AssertEqualInt(t, count, 1, "delete failed")
	t.Logf("Zone database:\n%s", zone)
	_, err = zone.LookupName(name1)
	wt.AssertErrorType(t, err, (*LookupError)(nil), "after deleting record")

	t.Logf("Checking if removing an unknown record results in an error")
	count = zone.DeleteRecords(container1, "", net.ParseIP("0.0.0.0"))
	wt.AssertEqualInt(t, count, 0, "delete failed")
}

func TestDeleteRecords(t *testing.T) {
	var (
		id    = "foobar"
		name  = "foo.weave."
		addr1 = "10.2.2.3/24"
		addr2 = "10.2.7.8/24"
	)

	InitDefaultLogging(testing.Verbose())

	zone, err := NewZoneDb(ZoneConfig{})
	wt.AssertNoErr(t, err)
	err = zone.Start()
	wt.AssertNoErr(t, err)
	defer zone.Stop()

	for _, addr := range []string{addr1, addr2} {
		ip, _, _ := net.ParseCIDR(addr)
		err := zone.AddRecord(id, name, ip)
		wt.AssertNoErr(t, err)
	}

	_, err = zone.LookupName(name)
	wt.AssertNoErr(t, err)

	count := zone.DeleteRecords(id, "", net.IP{})
	wt.AssertEqualInt(t, count, 2, "wildcard delete failed")
	_, err = zone.LookupName(name)
	wt.AssertErrorType(t, err, (*LookupError)(nil), "after deleting records for ident")
}
