package common

import (
	"crypto/rand"
	"crypto/sha256"
	"io/ioutil"
	"net"
)

func RandomMAC() (net.HardwareAddr, error) {
	mac := make([]byte, 6)
	if _, err := rand.Read(mac); err != nil {
		return nil, err
	}

	setUnicastAndLocal(mac)

	return net.HardwareAddr(mac), nil
}

func PersistentMAC() (net.HardwareAddr, error) {
	systemUUID, err := ioutil.ReadFile("/sys/class/dmi/id/product_uuid")
	if err != nil {
		return nil, err
	}

	hash := sha256.New()
	hash.Write([]byte("9oBJ0Jmip-"))
	hash.Write(systemUUID)
	sum := hash.Sum(nil)

	setUnicastAndLocal(sum)

	return net.HardwareAddr(sum[:6]), nil
}

func setUnicastAndLocal(mac []byte) {
	mac[0] = (mac[0] & 0xFE) | 0x02
}
