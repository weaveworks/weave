package nameserver

import (
	"github.com/miekg/dns"
	. "github.com/zettio/weave/common"
	"net"
)

func mdnsLookup(client *MDNSClient, name string, qtype uint16) (*ResponseA, error) {
	channel := make(chan *ResponseA)
	client.SendQuery(name, qtype, channel)
	for resp := range channel {
		Debug.Printf("Got response name %s addr %s", resp.Name, resp.Addr)
		if err := resp.Err; err != nil {
			return nil, err
		} else {
			return resp, nil
		}
	}
	return nil, LookupError(name)
}

func (client *MDNSClient) LookupLocal(name string) (net.IP, error) {
	if r, e := mdnsLookup(client, name, dns.TypeA); r != nil {
		return r.Addr, nil
	} else {
		return nil, e
	}
}

func (client *MDNSClient) ReverseLookupLocal(inaddr string) (string, error) {
	if r, e := mdnsLookup(client, inaddr, dns.TypePTR); r != nil {
		return r.Name, nil
	} else {
		return "", e
	}
}
