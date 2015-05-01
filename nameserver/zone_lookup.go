package nameserver

import (
	"github.com/miekg/dns"
	. "github.com/weaveworks/weave/common"
)

// Perform a lookup for a name in the zone
// The name can be resolved locally with the local database
func (zone *zoneDb) LookupName(name string) (res []ZoneRecord, err error) {
	zone.mx.RLock()
	defer zone.mx.RUnlock()

	// note: LookupName() is usually called from the mDNS server, so we do not touch the name
	name = dns.Fqdn(name)
	Debug.Printf("[zonedb] Looking for name '%s' in local database", name)
	for identName, nameset := range zone.idents {
		if identName != defaultRemoteIdent {
			for _, ze := range nameset.getEntriesForName(name) {
				res = append(res, ze)
			}
		}
	}

	if len(res) == 0 {
		err = LookupError(name)
	}
	return
}

// Perform a lookup for a IP address in the zone
// The address can be resolved locally with the local database
func (zone *zoneDb) LookupInaddr(inaddr string) (res []ZoneRecord, err error) {
	zone.mx.RLock()
	defer zone.mx.RUnlock()

	// note: LookupInaddr() is usually called from the mDNS server, so we do not touch the name

	revIPv4, err := raddrToIPv4(inaddr)
	if err != nil {
		return nil, newParseError("lookup address", inaddr)
	}
	Debug.Printf("[zonedb] Looking for address in local database: '%s' (%s)", revIPv4, inaddr)
	for identName, nameset := range zone.idents {
		if identName != defaultRemoteIdent {
			for _, ze := range nameset.getEntriesForIP(revIPv4) {
				res = append(res, ZoneRecord(ze))
			}
		}
	}
	if len(res) == 0 {
		err = LookupError(inaddr)
	}
	return
}
