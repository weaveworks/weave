package nameserver

import (
	"github.com/miekg/dns"
	. "github.com/weaveworks/weave/common"
)

func mdnsLookup(client *MDNSClient, name string, qtype uint16, insistent bool) ([]ZoneRecord, error) {
	var responses []*Response
	channel := make(chan *Response)
	client.SendQuery(name, qtype, insistent, channel)

	for resp := range channel {
		if err := resp.err; err != nil {
			Log.Debugf("[mdns] Error for query type %s name %s: %s", dns.TypeToString[qtype], name, err)
			return nil, err
		}
		Log.Debugf("[mdns] Got response name for %s-query about name '%s': name '%s', addr '%s'",
			dns.TypeToString[qtype], name, resp.name, resp.addr)

		if !insistent {
			return []ZoneRecord{resp}, nil // non-background queries return the first response...
		}

		// add the response, avoiding duplicates
		// TODO: replace this linear search by something better... (not a big deal as we don't expect long responses)
		duplicate := false
		for _, r := range responses {
			if r.Equal(resp) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			responses = append(responses, resp)
		}
	}

	if len(responses) == 0 {
		return nil, LookupError(name)
	}

	// convert []Response to []ZoneRecord
	res := make([]ZoneRecord, len(responses))
	for i, rr := range responses {
		res[i] = ZoneRecord(rr)
	}
	return res, nil
}

// Perform a lookup for a name, returning either an IP address or an error
func (client *MDNSClient) LookupName(name string) ([]ZoneRecord, error) {
	return mdnsLookup(client, name, dns.TypeA, false)
}

// Perform a lookup for an IP address, returning either a name or an error
func (client *MDNSClient) LookupInaddr(inaddr string) ([]ZoneRecord, error) {
	return mdnsLookup(client, inaddr, dns.TypePTR, false)
}

// Perform lookup for a name, returning either a list of IP addresses or an error
// Insistent queries will wait up to `mDNSTimeout` milliseconds for responses
func (client *MDNSClient) InsistentLookupName(name string) ([]ZoneRecord, error) {
	return mdnsLookup(client, name, dns.TypeA, true)
}

// Perform an insistent lookup for an IP address, returning either a list of names or an error
// Insistent queries will wait up to `mDNSTimeout` milliseconds for responses
func (client *MDNSClient) InsistentLookupInaddr(inaddr string) ([]ZoneRecord, error) {
	return mdnsLookup(client, inaddr, dns.TypePTR, true)
}
