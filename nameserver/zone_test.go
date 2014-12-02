package nameserver

import (
	"net"
	"testing"
)

func TestZone(t *testing.T) {
	var (
		containerID      = "deadbeef"
		otherContainerID = "cowjuice"
		successTestName  = "test1.weave."
		testAddr1        = "10.0.2.1/24"
	)

	var zone = new(ZoneDb)

	ip, _, _ := net.ParseCIDR(testAddr1)
	err := zone.AddRecord(containerID, successTestName, ip)
	assertNoErr(t, err)

	// Add a few more records to make the job harder
	err = zone.AddRecord("abcdef0123", "adummy.weave.", net.ParseIP("10.0.0.1"))
	assertNoErr(t, err)
	err = zone.AddRecord("0123abcdef", "zdummy.weave.", net.ParseIP("10.0.0.2"))
	assertNoErr(t, err)

	// Check that the address is now there.
	foundIP, err := zone.LookupLocal(successTestName)
	assertNoErr(t, err)

	if !foundIP.Equal(ip) {
		t.Fatal("Unexpected result for", successTestName, foundIP)
	}

	// See if we can find the address by IP.
	foundName, err := zone.ReverseLookupLocal(ip)
	assertNoErr(t, err)

	if foundName != successTestName {
		t.Fatal("Unexpected result for", ip, foundName)
	}

	err = zone.AddRecord(containerID, successTestName, ip)
	assertErrorType(t, err, (*DuplicateError)(nil), "duplicate add")

	err = zone.AddRecord(otherContainerID, successTestName, ip)
	// Delete the record for the original container
	err = zone.DeleteRecord(containerID, ip)
	assertNoErr(t, err)

	_, err = zone.LookupLocal(successTestName)
	assertNoErr(t, err)

	err = zone.DeleteRecord(otherContainerID, ip)
	assertNoErr(t, err)

	// Check that the address is not there now.
	_, err = zone.LookupLocal(successTestName)
	assertErrorType(t, err, (*LookupError)(nil), "after deleting record")

	// Delete a record that isn't there
	err = zone.DeleteRecord(containerID, net.ParseIP("0.0.0.0"))
	assertErrorType(t, err, (*LookupError)(nil), "when deleting record that doesn't exist")
}

func TestDeleteFor(t *testing.T) {
	var (
		id    = "foobar"
		name  = "foo.weave."
		addr1 = "10.1.2.3/24"
		addr2 = "10.2.7.8/24"
	)
	zone := new(ZoneDb)
	for _, addr := range []string{addr1, addr2} {
		ip, _, _ := net.ParseCIDR(addr)
		err := zone.AddRecord(id, name, ip)
		assertNoErr(t, err)
	}

	_, err := zone.LookupLocal(name)
	assertNoErr(t, err)

	err = zone.DeleteRecordsFor(id)
	_, err = zone.LookupLocal(name)
	assertErrorType(t, err, (*LookupError)(nil), "after deleting records for ident")
}
