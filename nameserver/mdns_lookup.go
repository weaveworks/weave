package nameserver

import (
	"github.com/miekg/dns"
	. "github.com/zettio/weave/common"
	"net"
)

func mdnsLookup(client *MDNSClient, name string, qtype uint16) (*Response, error) {
	channel := make(chan *Response)
	client.SendQuery(name, qtype, channel)
	for resp := range channel {
		if err := resp.Err; err != nil {
			Debug.Printf("[mdns] Error for query type %s name %s: %s",
				dns.TypeToString[qtype], name, err)
			return nil, err
		}
		Debug.Printf("[mdns] Got response name for query type %s name %s: name %s, addr %s",
			dns.TypeToString[qtype], name, resp.Name, resp.Addr)
		return resp, nil
	}
	return nil, LookupError(name)
}

func (client *MDNSClient) LookupName(name string) (net.IP, error) {
	if r, e := mdnsLookup(client, name, dns.TypeA); r != nil {
		return r.Addr, nil
	} else {
		return nil, e
	}
}

func (client *MDNSClient) LookupInaddr(inaddr string) (string, error) {
	if r, e := mdnsLookup(client, inaddr, dns.TypePTR); r != nil {
		return r.Name, nil
	} else {
		return "", e
	}
}
