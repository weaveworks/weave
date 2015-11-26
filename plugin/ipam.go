package plugin

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"

	. "github.com/weaveworks/weave/common"
)

func (d *dockerer) ipamOp(ID string, op string) (*net.IPNet, error) {
	weaveip, err := d.getContainerBridgeIP(WeaveContainer)
	Log.Debugf("IPAM operation %s for %s", op, ID)
	if err != nil {
		return nil, err
	}

	var res *http.Response

	url := fmt.Sprintf("http://%s:6784/ip/%s", weaveip, ipamID(ID))
	Log.Debugf("Attempting to %s to %s", op, url)
	if op == "POST" {
		res, err = http.Post(url, "", nil)
	} else if op == "GET" {
		res, err = http.Get(url)
	}

	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received status %d from IPAM", res.StatusCode)
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	return parseIP(string(body))
}

// returns an IP for the ID given, allocating a fresh one if necessary
func (d *dockerer) allocateIP(ID string) (*net.IPNet, error) {
	return d.ipamOp(ID, "POST")
}

// returns an IP for the ID given, or nil if one has not been
// allocated
func (d *dockerer) lookupIP(ID string) (*net.IPNet, error) {
	return d.ipamOp(ID, "GET")
}

// release an IP which is no longer needed
func (d *dockerer) releaseIP(ID string) error {
	weaveip, err := d.getContainerBridgeIP(WeaveContainer)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("DELETE", fmt.Sprintf("http://%s:6784/ip/%s", weaveip, ipamID(ID)), nil)
	if err != nil {
		return err
	}
	cl := &http.Client{}
	res, err := cl.Do(req)
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("unexpected HTTP status code from IP release: %d", res.StatusCode)
	}
	return nil
}

func parseIP(body string) (*net.IPNet, error) {
	ip, ipnet, err := net.ParseCIDR(string(body))
	if err != nil {
		return nil, err
	}
	ipnet.IP = ip
	return ipnet, nil
}

// If something looking like a container ID is supplied to IPAM, it
// will try to check that it's running, and return nothing (and empty
// string) if it's not. So I make sure it doesn't look like a
// container ID by including non-hex characters.
func ipamID(ID string) string {
	return "endpoint:" + ID
}
