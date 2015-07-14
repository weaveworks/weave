package nameserver

import (
	"net"
	"time"

	"github.com/miekg/dns"
	. "github.com/weaveworks/weave/common"
)

type uniqZoneRecordKey struct {
	name string
	ipv4 IPv4
}

// A group of ZoneRecords where there are no duplicates (according to the name & IPv4)
type uniqZoneRecords map[uniqZoneRecordKey]ZoneRecord

func newUniqZoneRecords() uniqZoneRecords {
	return make(uniqZoneRecords, 0)
}

// Add a new ZoneRecord to the group
func (uzr *uniqZoneRecords) add(zr ZoneRecord) {
	key := uniqZoneRecordKey{zr.Name(), ipToIPv4(zr.IP())}
	(*uzr)[key] = zr
}

// Return the group as an slice
func (uzr *uniqZoneRecords) toSlice() []ZoneRecord {
	res := make([]ZoneRecord, len(*uzr))
	i := 0
	for _, r := range *uzr {
		res[i] = r
		i++
	}
	return res
}

//////////////////////////////////////////////////////////////////////////////

// Lookup in the database for locally-introduced information
func (zone *ZoneDb) lookup(target string, lfun func(ns *nameSet) []*recordEntry) (res []ZoneRecord, err error) {
	uniq := newUniqZoneRecords()
	for identName, nameset := range zone.idents {
		if identName != defaultRemoteIdent {
			for _, ze := range lfun(nameset) {
				uniq.add(ze)
			}
		}
	}
	if len(uniq) == 0 {
		return nil, LookupError(target)
	}
	return uniq.toSlice(), nil
}

// Perform a lookup for a name in the zone
// The name can be resolved locally with the local database
func (zone *ZoneDb) LookupName(name string) (res []ZoneRecord, err error) {
	zone.mx.RLock()
	defer zone.mx.RUnlock()

	// note: LookupName() is usually called from the mDNS server, so we do not touch the name
	name = dns.Fqdn(name)
	Log.Debugf("[zonedb] Looking for name '%s' in local database", name)
	return zone.lookup(name, func(ns *nameSet) []*recordEntry { return ns.getEntriesForName(name) })
}

// Perform a lookup for a IP address in the zone
// The address can be resolved locally with the local database
func (zone *ZoneDb) LookupInaddr(inaddr string) (res []ZoneRecord, err error) {
	zone.mx.RLock()
	defer zone.mx.RUnlock()

	// note: LookupInaddr() is usually called from the mDNS server, so we do not touch the name
	revIPv4, err := raddrToIPv4(inaddr)
	if err != nil {
		return nil, newParseError("lookup address", inaddr)
	}
	Log.Debugf("[zonedb] Looking for address in local database: '%s' (%s)", revIPv4, inaddr)
	return zone.lookup(inaddr, func(ns *nameSet) []*recordEntry { return ns.getEntriesForIP(revIPv4) })
}

// Perform a domain lookup with mDNS
func (zone *ZoneDb) domainLookup(target string, lfun ZoneLookupFunc) (res []ZoneRecord, err error) {
	// no local results have been obtained in the local database: try with a mDNS query
	Log.Debugf("[zonedb] '%s' not in local database... trying with mDNS", target)
	lanswers, err := lfun(target)
	if err != nil {
		Log.Debugf("[zonedb] mDNS lookup error for '%s': %s", target, err)
		return nil, err
	}

	// if the request has been successful, save the IP in the local database and return the corresponding ZoneRecord
	// (we do not get the remote ident in the mDNS reply, so we save it in a "remote" ident)
	Log.Debugf("[zonedb] adding '%s' (obtained with mDNS) to '%s'", lanswers, target)
	res = make([]ZoneRecord, len(lanswers))
	zone.mx.Lock()
	now := zone.clock.Now()
	uniq := newUniqZoneRecords()
	remoteIdent := zone.getNameSet(defaultRemoteIdent)
	for _, answer := range lanswers {
		r, err := remoteIdent.addIPToName(answer, now)
		if err != nil {
			zone.mx.Unlock()
			Log.Warningf("[zonedb] '%s' insertion for %s failed: %s", answer, target, err)
			return nil, err
		}
		uniq.add(r)
	}
	zone.mx.Unlock()

	return uniq.toSlice(), nil
}

// Perform a lookup for a name in the zone
// The name can be resolved locally with the local database or with some other resolution method (eg, a mDNS query)
func (zone *ZoneDb) DomainLookupName(name string) (res []ZoneRecord, err error) {
	name = dns.Fqdn(name)
	Log.Debugf("[zonedb] Looking for name '%s' in local(&remote) database", name)

	zone.mx.Lock()
	now := zone.clock.Now()
	uniq := newUniqZoneRecords()
	for identName, nameset := range zone.idents {
		for _, ze := range nameset.getEntriesForName(name) {
			// filter the entries with expired TTL
			// locally introduced entries are never expired: they always have TTL=0
			if ze.hasExpired(now) {
				Log.Debugf("[zonedb] '%s': expired entry '%s' ignored: removing", name, ze)
				nameset.deleteNameIP(name, net.IP{})
			} else {
				uniq.add(ze)
			}
		}
		if identName != defaultRemoteIdent {
			nameset.touchName(name, now)
		}
	}
	zone.mx.Unlock()

	if len(uniq) > 0 {
		Log.Debugf("[zonedb] '%s' resolved in local database", name)
		res = uniq.toSlice()
	} else {
		res, err = zone.domainLookup(name, zone.mdnsCli.LookupName)
	}

	if len(res) > 0 {
		zone.startUpdatingName(name)
		return res, nil
	}
	return nil, LookupError(name)
}

// Perform a lookup for a IP address in the zone
// The address can be resolved either with the local database or
// with some other resolution method (eg, a mDNS query)
func (zone *ZoneDb) DomainLookupInaddr(inaddr string) (res []ZoneRecord, err error) {
	revIPv4, err := raddrToIPv4(inaddr)
	if err != nil {
		return nil, newParseError("lookup address", inaddr)
	}

	Log.Debugf("[zonedb] Looking for address in local(&remote) database: '%s' (%s)", revIPv4, inaddr)

	zone.mx.Lock()
	now := zone.clock.Now()
	uniq := newUniqZoneRecords()
	for identName, nameset := range zone.idents {
		for _, ze := range nameset.getEntriesForIP(revIPv4) {
			// filter the entries with expired TTL
			// locally introduced entries are never expired: they always have TTL=0
			if ze.hasExpired(now) {
				Log.Debugf("[zonedb] '%s': expired entry '%s' ignored: removing", revIPv4, ze)
				nameset.deleteNameIP("", revIPv4.toNetIP())
			} else {
				uniq.add(ze)
				if identName != defaultRemoteIdent {
					nameset.touchName(ze.Name(), now)
				}
			}
		}
	}
	zone.mx.Unlock()

	if len(uniq) > 0 {
		Log.Debugf("[zonedb] '%s' resolved in local database", inaddr)
		res = uniq.toSlice()
	} else {
		res, err = zone.domainLookup(inaddr, zone.mdnsCli.LookupInaddr)
	}

	if len(res) > 0 {
		// note: even for reverse addresses, we perform the background updates in the name, not in the IP
		//       this simplifies the process and produces basically the same results...
		// note: we do not spend time trying to update names that did not return an initial response...
		for _, r := range res {
			zone.startUpdatingName(r.Name())
		}

		return res, nil
	}
	return nil, LookupError(inaddr)
}

//////////////////////////////////////////////////////////////////////////////

// Names updates try to find all the IPs for a given name with a mDNS query
//
// There are two types of names updates:
//
// - immediate updates.
//   After a `DomainLookup*()` for a name not in the database we will return the
//   first IP we can get with mDNS from other peers. Waiting for more responses would
//   mean more latency in the response to the client, so we send only one answer BUT
//   we also trigger an immediate update request for that name in order to get all
//   the other IPs we didn't wait for...
//
// - periodic updates
//   once we have obtained the first group of IPs for a name, we schedule a periodic
//   refresh for that name, so we keep the list of IPs for that name up to date.
//
// These names updates are repeated until either
//
//  a) there is no interest in the name, determined by a global 'relevant time'
//     and the last time some local client asked about the name,
//     or
//  b) no peers answer one of our refresh requests (because the name has probably
//     disappeared from the network)
//

// Check if we must start updating a name and, in that case, trigger a immediate update
func (zone *ZoneDb) startUpdatingName(name string) {
	if zone.refreshInterval > 0 {
		zone.mx.Lock()

		// check if we should enqueue a refresh request for this name
		n := zone.getNameSet(defaultRemoteIdent).getName(name, true)
		if n.lastRefreshTime.IsZero() {
			now := zone.clock.Now()
			n.lastRefreshTime = now
			zone.mx.Unlock()

			Log.Debugf("[zonedb] Creating new immediate refresh request for '%s'", name)
			zone.refreshScheds.Add(func() time.Time { return zone.updater(name) }, now)
		} else {
			zone.mx.Unlock()
		}
	}
}

// Update the IPs we have for a name
func (zone *ZoneDb) updater(name string) (nextTime time.Time) {
	deleteRemoteInfo := func() {
		zone.mx.Lock()
		zone.getNameSet(defaultRemoteIdent).deleteNameIP(name, net.IP{})
		zone.mx.Unlock()
	}

	// if nobody has asked for this name for long time, just forget about it...
	if !zone.IsNameRelevant(name) || zone.IsNameExpired(name) {
		Log.Debugf("[zonedb] '%s' seem to be irrelevant now: removing any remote information", name)
		deleteRemoteInfo()
		return
	}

	// perform the refresh for this name
	fullName := dns.Fqdn(name)
	startTime := zone.clock.Now()
	Log.Debugf("[zonedb] Refreshing name '%s' with mDNS...", fullName)
	res, _ := zone.mdnsCli.InsistentLookupName(fullName)
	if res != nil && len(res) > 0 {
		numIps := len(res)
		zone.mx.Lock()
		now := zone.clock.Now()
		added, removed := zone.getNameSet(defaultRemoteIdent).getName(name, true).updateIPs(res, now)
		zone.mx.Unlock()
		Log.Debugf("[zonedb] Obtained %d IPs for name '%s' with mDNS: %d added, %d removed",
			numIps, name, added, removed)

		// once the name has been updated, we re-schedule the update
		nextTime = startTime.Add(zone.refreshInterval)
		Log.Debugf("[zonedb] Rescheduling update for '%s' in %s", name, nextTime.Sub(zone.clock.Now()))
	} else {
		Log.Debugf("[zonedb] nobody knows about '%s'... removing", name)
		deleteRemoteInfo()
	}
	return
}
