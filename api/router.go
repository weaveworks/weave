package api

import (
	"fmt"
	"net"
	"net/url"
)

// Expose calls the router to assign the given IP addr to the weave bridge.
//
// The k8s param denotes whether Weave Net is running on (for) Kubernetes.
func (client *Client) Expose(ipAddr *net.IPNet, k8s bool) error {
	values := make(url.Values)
	if k8s {
		values.Set("k8s", "true")
	}
	_, err := client.httpVerb("POST", fmt.Sprintf("/expose/%s", ipAddr), values)
	return err
}
