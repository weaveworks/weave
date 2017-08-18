package api

import (
	"fmt"
	"net"
)

// Expose calls the router to assign the given IP addr to the weave bridge.
func (client *Client) Expose(ipAddr *net.IPNet) error {
	_, err := client.httpVerb("POST", fmt.Sprintf("/expose/%s", ipAddr), nil)
	return err
}
