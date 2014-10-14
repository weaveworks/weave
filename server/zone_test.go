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
	if err != nil {
		t.Fatal(err)
	}

	// Check that the address is now there.
	found_ip, err := zone.MatchLocal(success_test_name)
	if err != nil {
		t.Fatal(err)
	}
	if !found_ip.Equal(weave_ip) {
		t.Fatal("Unexpected result for", success_test_name, ip)
	}

	// Now try to add the same address again
	err = zone.AddRecord(container_id, success_test_name, ip, weave_ip, subnet)
	if _, ok := err.(DuplicateError); !ok {
		t.Fatal(err)
	}
}
