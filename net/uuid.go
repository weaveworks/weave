package net

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
)

func getSystemUUID(hostRoot string) ([]byte, error) {
	machineid, err := ioutil.ReadFile(hostRoot + "/etc/machine-id")
	if os.IsNotExist(err) {
		machineid, _ = ioutil.ReadFile(hostRoot + "/var/lib/dbus/machine-id")
	}
	uuid, err := ioutil.ReadFile(hostRoot + "/sys/class/dmi/id/product_uuid")
	if os.IsNotExist(err) {
		uuid, _ = ioutil.ReadFile(hostRoot + "/sys/hypervisor/uuid")
	}
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if len(machineid)+len(uuid) == 0 {
		return nil, fmt.Errorf("Empty system uuid")
	}
	return append(machineid, uuid...), nil
}

// GetSystemPeerName returns an ID derived from concatenated machine-id
// (either systemd or dbus), the system (aka bios) UUID and the
// hypervisor UUID.  It is tweaked and formatted to be usable as a mac address
func GetSystemPeerName(hostRoot string) (string, error) {
	var mac net.HardwareAddr
	if uuid, err := getSystemUUID(hostRoot); err == nil {
		mac = MACfromUUID(uuid)
	} else {
		mac, err = RandomMAC()
		if err != nil {
			return "", err
		}
	}
	return mac.String(), nil
}
