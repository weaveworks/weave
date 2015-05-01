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

// Perform a lookup for a name in the zone
// The name can be resolved locally with the local database or with some other resolution method (eg, a mDNS query)
func (zone *zoneDb) DomainLookupName(name string) (res []ZoneRecord, err error) {
	name = dns.Fqdn(name)
	Debug.Printf("[zonedb] Looking for name '%s' in local(&remote) database", name)

	zone.mx.RLock()
	now := zone.clock.Now()
	for identName, nameset := range zone.idents {
		for _, ze := range nameset.getEntriesForName(name) {
			// filter the entries with expired TTL
			// locally introduced entries are nver expired as the always have TTL=0
			if ze.hasExpired(now) {
				Debug.Printf("[zonedb] '%s': expired entry '%s' ignored: removing", name, ze)
				nameset.deleteName(name)
			} else {
				res = append(res, ze)
			}
		}
		if identName != defaultRemoteIdent {
			nameset.touchName(name, now)
		}
	}
	zone.mx.RUnlock()

	if len(res) > 0 {
		Debug.Printf("[zonedb] '%s' resolved in local database", name)
	} else {
		// no local results have been obtained in the local database: try with a mDNS query
		// (this request is not a background query, so we cannot delay the response)
		Debug.Printf("[zonedb] name '%s' not in local database: trying with mDNS", name)
		ips, err := zone.mdnsCli.LookupName(name)
		if err != nil {
			Debug.Printf("[zonedb] mDNS lookup error for '%s': %s", name, err)
			return nil, err
		}

		// if the request has been successful, save the IP in the local database and return the corresponding ZoneRecord
		// (we do not get the remote ident in the mDNS reply, so we save it in a "remote" ident)
		Debug.Printf("[zonedb] adding '%s' (obtained with mDNS) to '%s'", ips, name)
		res = make([]ZoneRecord, len(ips))
		zone.mx.Lock()
		now = zone.clock.Now()
		for i, zr := range ips {
			res[i], err = zone.getNamesSet(defaultRemoteIdent).addIPToName(zr, now)
			if err != nil {
				zone.mx.Unlock()
				Warning.Printf("[zonedb] IP [%s] insertion for '%s' failed: %s", ips, name, err)
				return nil, err
			}
		}
		zone.mx.Unlock()
	}

	if len(res) > 0 {
		return res, nil
	}

	return nil, LookupError(name)
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

// Perform a lookup for a IP address in the zone
// The address can be resolved either with the local database or
// with some other resolution method (eg, a mDNS query)
func (zone *zoneDb) DomainLookupInaddr(inaddr string) (res []ZoneRecord, err error) {
	revIPv4, err := raddrToIPv4(inaddr)
	if err != nil {
		return nil, newParseError("lookup address", inaddr)
	}

	Debug.Printf("[zonedb] Looking for address in local(&remote) database: '%s' (%s)", revIPv4, inaddr)

	zone.mx.RLock()
	now := zone.clock.Now()
	for identName, nameset := range zone.idents {
		for _, ze := range nameset.getEntriesForIP(revIPv4) {
			// filter the entries with expired TTL
			// locally introduced entries are nver expired as the always have TTL=0
			if ze.hasExpired(now) {
				Debug.Printf("[zonedb] '%s': expired entry '%s' ignored: removing", revIPv4, ze)
				nameset.deleteIP(revIPv4)
			} else {
				res = append(res, ZoneRecord(ze))
				if identName != defaultRemoteIdent {
					nameset.touchName(ze.Name(), now)
				}
			}
		}
	}
	zone.mx.RUnlock()

	if len(res) > 0 {
		Debug.Printf("[zonedb] '%s' resolved in local database", inaddr)
	} else {
		// no local results have been obtained in the local database: try with a mDNS query
		Debug.Printf("[zonedb] '%s'(%+v) not in local database... trying with mDNS", inaddr, revIPv4)
		names, err := zone.mdnsCli.LookupInaddr(inaddr)
		if err != nil {
			Debug.Printf("[zonedb] mDNS lookup error for '%s': %s", inaddr, err)
			return nil, err
		}

		// if the request has been successful, save the IP in the local database and return the corresponding ZoneRecord
		// (we do not get the remote ident in the mDNS reply, so we save it in a "remote" ident)
		Debug.Printf("[zonedb] adding '%s' (obtained with mDNS) to '%s'", names, revIPv4)
		res = make([]ZoneRecord, len(names))
		zone.mx.Lock()
		now = zone.clock.Now()
		for i, name := range names {
			res[i], err = zone.getNamesSet(defaultRemoteIdent).addIPToName(name, now)
			if err != nil {
				zone.mx.Unlock()
				Warning.Printf("[zonedb] Name '%s' insertion for %s failed: %s", name.Name(), revIPv4, err)
				return nil, err
			}
		}
		zone.mx.Unlock()
	}

	if len(res) > 0 {
		return res, nil
	}

	return nil, LookupError(inaddr)

}

