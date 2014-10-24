package weavedns

import (
	"net"
	"testing"
)

func TestZone(t *testing.T) {
	var (
		containerID     = "deadbeef"
		successTestName = "test1.weave."
		testAddr1       = "10.0.2.1/24"
		dockerIP        = "9.8.7.6"
	)

	var zone = new(ZoneDb)

	ip := net.ParseIP(dockerIP)
	weaveIP, _, _ := net.ParseCIDR(testAddr1)
	err := zone.AddRecord(containerID, successTestName, ip, weaveIP)
	assertNoErr(t, err)

	// Add a few more records to make the job harder
	err = zone.AddRecord("abcdef0123", "adummy.weave.", net.ParseIP("1.1.1.1"), net.ParseIP("10.0.0.1"))
	assertNoErr(t, err)
	err = zone.AddRecord("0123abcdef", "zdummy.weave.", net.ParseIP("2.2.2.2"), net.ParseIP("10.0.0.2"))
	assertNoErr(t, err)

	// Check that the address is now there.
	foundIP, err := zone.MatchLocal(successTestName)
	assertNoErr(t, err)

	if !foundIP.Equal(weaveIP) {
		t.Fatal("Unexpected result for", successTestName, foundIP)
	}

	// Now try to add the same address again
	err = zone.AddRecord(containerID, successTestName, ip, weaveIP)
	assertErrorType(t, err, (*DuplicateError)(nil), "duplicate add")

	// Now delete the record
	err = zone.DeleteRecord(containerID, weaveIP)
	assertNoErr(t, err)

	// Check that the address is not there now.
	_, err = zone.MatchLocal(successTestName)
	assertErrorType(t, err, (*LookupError)(nil), "after deleting record")

	// Delete a record that isn't there
	err = zone.DeleteRecord(containerID, net.ParseIP("0.0.0.0"))
	assertErrorType(t, err, (*LookupError)(nil), "when deleting record that doesn't exist")
}

func TestDeleteFor(t *testing.T) {
	var (
		id       = "foobar"
		name     = "foo.weave."
		addr1    = "10.1.2.3/24"
		addr2    = "10.2.7.8/24"
		dockerIP = "172.16.0.4"
	)
	zone := new(ZoneDb)
	ip := net.ParseIP(dockerIP)
	for _, addr := range []string{addr1, addr2} {
		weaveIP, _, _ := net.ParseCIDR(addr)
		err := zone.AddRecord(id, name, ip, weaveIP)
		assertNoErr(t, err)
	}

	_, err := zone.MatchLocal(name)
	assertNoErr(t, err)

	err = zone.DeleteRecordsFor(id)
	_, err = zone.MatchLocal(name)
	assertErrorType(t, err, (*LookupError)(nil), "after deleting records for ident")
}
