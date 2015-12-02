package api

import (
	"fmt"
	"net/url"
)

func (client *Client) RegisterWithDNS(ID string, fqdn string, ip string) error {
	data := url.Values{}
	data.Add("fqdn", fqdn)
	_, err := httpVerb("PUT", fmt.Sprintf("%s/name/%s/%s", client.baseUrl, ID, ip), data)
	return err
}

func (client *Client) DeregisterWithDNS(ID string, ip string) error {
	_, err := httpVerb("DELETE", fmt.Sprintf("%s/name/%s/%s", client.baseUrl, ID, ip), nil)
	return err
}
