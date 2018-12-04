package api

import (
	"fmt"
	"net"
	"net/url"
)

// Expose calls the router to assign the given IP addr to the weave bridge.
func (client *Client) Expose(ipAddr *net.IPNet) error {
	_, err := client.httpVerb("POST", fmt.Sprintf("/expose/%s", ipAddr), nil)
	return err
}

// ReplacePeers replace the current set of peers
func (client *Client) ReplacePeers(peers []string) error {
	_, err := client.httpVerb("POST", "/connect", url.Values{"replace": {"true"}, "peer": peers})
	return err
}
