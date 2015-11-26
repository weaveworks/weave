package plugin

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func (d *dockerer) registerWithDNS(ID string, fqdn string, ip string) error {
	dnsip, err := d.getContainerBridgeIP(WeaveDNSContainer)
	if err != nil {
		return fmt.Errorf("nameserver not available: %s", err)
	}
	data := url.Values{}
	data.Add("fqdn", fqdn)

	req, err := http.NewRequest("PUT", fmt.Sprintf("http://%s:6785/name/%s/%s", dnsip, ID, ip), strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	cl := &http.Client{}
	res, err := cl.Do(req)
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("non-OK status from nameserver: %d", res.StatusCode)
	}
	return nil
}

func (d *dockerer) deregisterWithDNS(ID string, ip string) error {
	dnsip, err := d.getContainerBridgeIP(WeaveDNSContainer)
	if err != nil {
		return fmt.Errorf("nameserver not available: %s", err)
	}

	req, err := http.NewRequest("DELETE", fmt.Sprintf("http://%s:6785/name/%s/%s", dnsip, ID, ip), nil)
	if err != nil {
		return err
	}

	cl := &http.Client{}
	res, err := cl.Do(req)
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("non-OK status from nameserver: %d", res.StatusCode)
	}
	return nil
}
