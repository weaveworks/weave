package nameserver

import (
	wt "github.com/weaveworks/weave/testing"
	"net"
	"testing"
)

func TestZone(t *testing.T) {
	var (
		containerID      = "deadbeef"
		otherContainerID = "cowjuice"
		successTestName  = "test1.weave."
		testAddr1        = "10.2.2.1/24"
	)

	var zone = NewZoneDb(DefaultLocalDomain)

	ip, _, _ := net.ParseCIDR(testAddr1)
	err := zone.AddRecord(containerID, successTestName, ip)
	wt.AssertNoErr(t, err)

	// Add a few more records to make the job harder
	err = zone.AddRecord("abcdef0123", "adummy.weave.", net.ParseIP("10.2.0.1"))
	wt.AssertNoErr(t, err)
	err = zone.AddRecord("0123abcdef", "zdummy.weave.", net.ParseIP("10.2.0.2"))
	wt.AssertNoErr(t, err)

	// Check that the address is now there.
	foundIP, err := zone.LookupName(successTestName)
	wt.AssertNoErr(t, err)

	if !foundIP[0].IP().Equal(ip) {
		t.Fatal("Unexpected result for", successTestName, foundIP)
	}

	// See if we can find the address by IP.
	foundName, err := zone.LookupInaddr("1.2.2.10.in-addr.arpa.")
	wt.AssertNoErr(t, err)

	if foundName[0].Name() != successTestName {
		t.Fatal("Unexpected result for", ip, foundName)
	}

	err = zone.AddRecord(containerID, successTestName, ip)
	wt.AssertErrorType(t, err, (*DuplicateError)(nil), "duplicate add")

	err = zone.AddRecord(otherContainerID, successTestName, ip)
	// Delete the record for the original container
	err = zone.DeleteRecord(containerID, ip)
	wt.AssertNoErr(t, err)

	_, err = zone.LookupName(successTestName)
	wt.AssertNoErr(t, err)

	err = zone.DeleteRecord(otherContainerID, ip)
	wt.AssertNoErr(t, err)

	// Check that the address is not there now.
	_, err = zone.LookupName(successTestName)
	wt.AssertErrorType(t, err, (*LookupError)(nil), "after deleting record")

	// Delete a record that isn't there
	err = zone.DeleteRecord(containerID, net.ParseIP("0.0.0.0"))
	wt.AssertErrorType(t, err, (*LookupError)(nil), "when deleting record that doesn't exist")
}

func TestDeleteFor(t *testing.T) {
	var (
		id    = "foobar"
		name  = "foo.weave."
		addr1 = "10.2.2.3/24"
		addr2 = "10.2.7.8/24"
	)
	zone := NewZoneDb(DefaultLocalDomain)
	for _, addr := range []string{addr1, addr2} {
		ip, _, _ := net.ParseCIDR(addr)
		err := zone.AddRecord(id, name, ip)
		wt.AssertNoErr(t, err)
	}

	_, err := zone.LookupName(name)
	wt.AssertNoErr(t, err)

	err = zone.DeleteRecordsFor(id)
	_, err = zone.LookupName(name)
	wt.AssertErrorType(t, err, (*LookupError)(nil), "after deleting records for ident")
}
