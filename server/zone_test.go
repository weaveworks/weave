package weavedns

import (
	"net"
	"testing"
)

func TestZone(t *testing.T) {
	var (
		container_id      = "deadbeef"
		success_test_name = "test1.weave."
		test_addr1        = "10.0.2.1/24"
		docker_ip         = "9.8.7.6"
	)

	var zone = new(ZoneDb)

	ip := net.ParseIP(docker_ip)
	weave_ip, subnet, _ := net.ParseCIDR(test_addr1)
	err := zone.AddRecord(container_id, success_test_name, ip, weave_ip, subnet)
	assertNoErr(t, err)

	// Check that the address is now there.
	found_ip, err := zone.MatchLocal(success_test_name)
	assertNoErr(t, err)

	if !found_ip.Equal(weave_ip) {
		t.Fatal("Unexpected result for", success_test_name, ip)
	}

	// Now try to add the same address again
	err = zone.AddRecord(container_id, success_test_name, ip, weave_ip, subnet)
	if _, ok := err.(DuplicateError); !ok {
		t.Fatal(err)
	}

	// Now delete the record
	err = zone.DeleteRecord(container_id, weave_ip)
	assertNoErr(t, err)

	// Check that the address is not there now.
	_, err = zone.MatchLocal(success_test_name)
	if _, ok := err.(LookupError); !ok {
		t.Fatal("Unexpected result when deleting record", success_test_name, err)
	}

	// Delete a record that isn't there
	err = zone.DeleteRecord(container_id, net.ParseIP("0.0.0.0"))
	if _, ok := err.(LookupError); !ok {
		t.Fatal("Expected a LookupError when deleting a record that doesn't exist")
	}
}

func TestDeleteFor(t *testing.T) {
	var (
		id        = "foobar"
		name      = "foo.weave."
		addr1     = "10.1.2.3/24"
		addr2     = "10.2.7.8/24"
		docker_ip = "172.16.0.4"
	)
	zone := new(ZoneDb)
	ip := net.ParseIP(docker_ip)
	for _, addr := range []string{addr1, addr2} {
		weave_ip, subnet, _ := net.ParseCIDR(addr)
		err := zone.AddRecord(id, name, ip, weave_ip, subnet)
		assertNoErr(t, err)
	}

	_, err := zone.MatchLocal(name)
	assertNoErr(t, err)

	err = zone.DeleteRecordsFor(id)
	_, err = zone.MatchLocal(name)
	if _, ok := err.(LookupError); !ok {
		t.Fatal("Expected no results after deleting records for ident")
	}
}
