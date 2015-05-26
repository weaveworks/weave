package nameserver

import (
	"github.com/miekg/dns"
	. "github.com/weaveworks/weave/common"
	"time"
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
func (zone *zoneDb) lookup(target string, lfun func(ns *namesSet) []*recordEntry) (res []ZoneRecord, err error) {
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
	} else {
		return uniq.toSlice(), nil
	}
}

// Perform a lookup for a name in the zone
// The name can be resolved locally with the local database
func (zone *zoneDb) LookupName(name string) (res []ZoneRecord, err error) {
	zone.mx.RLock()
	defer zone.mx.RUnlock()

	// note: LookupName() is usually called from the mDNS server, so we do not touch the name
	name = dns.Fqdn(name)
	Debug.Printf("[zonedb] Looking for name '%s' in local database", name)
	return zone.lookup(name, func(ns *namesSet) []*recordEntry { return ns.getEntriesForName(name) })
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
	return zone.lookup(inaddr, func(ns *namesSet) []*recordEntry { return ns.getEntriesForIP(revIPv4) })
}

// Perform a domain lookup with mDNS
func (zone *zoneDb) domainLookup(target string, lfun ZoneLookupFunc) (res []ZoneRecord, err error) {
	// no local results have been obtained in the local database: try with a mDNS query
	Debug.Printf("[zonedb] '%s' not in local database... trying with mDNS", target)
	lanswers, err := lfun(target)
	if err != nil {
		Debug.Printf("[zonedb] mDNS lookup error for '%s': %s", target, err)
		return nil, err
	}

	// if the request has been successful, save the IP in the local database and return the corresponding ZoneRecord
	// (we do not get the remote ident in the mDNS reply, so we save it in a "remote" ident)
	Debug.Printf("[zonedb] adding '%s' (obtained with mDNS) to '%s'", lanswers, target)
	res = make([]ZoneRecord, len(lanswers))
	zone.mx.Lock()
	now := zone.clock.Now()
	uniq := newUniqZoneRecords()
	for _, answer := range lanswers {
		r, err := zone.getNamesSet(defaultRemoteIdent).addIPToName(answer, now)
		if err != nil {
			zone.mx.Unlock()
			Warning.Printf("[zonedb] '%s' insertion for %s failed: %s", answer, target, err)
			return nil, err
		}
		uniq.add(r)
	}
	zone.mx.Unlock()

	return uniq.toSlice(), nil
}

// Perform a lookup for a name in the zone
// The name can be resolved locally with the local database or with some other resolution method (eg, a mDNS query)
func (zone *zoneDb) DomainLookupName(name string) (res []ZoneRecord, err error) {
	name = dns.Fqdn(name)
	Debug.Printf("[zonedb] Looking for name '%s' in local(&remote) database", name)

	zone.mx.RLock()
	now := zone.clock.Now()
	uniq := newUniqZoneRecords()
	for identName, nameset := range zone.idents {
		for _, ze := range nameset.getEntriesForName(name) {
			// filter the entries with expired TTL
			// locally introduced entries are never expired: they always have TTL=0
			if ze.hasExpired(now) {
				Debug.Printf("[zonedb] '%s': expired entry '%s' ignored: removing", name, ze)
				nameset.deleteName(name)
			} else {
				uniq.add(ze)
			}
		}
		if identName != defaultRemoteIdent {
			nameset.touchName(name, now)
		}
	}
	zone.mx.RUnlock()

	if len(uniq) > 0 {
		Debug.Printf("[zonedb] '%s' resolved in local database", name)
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
func (zone *zoneDb) DomainLookupInaddr(inaddr string) (res []ZoneRecord, err error) {
	revIPv4, err := raddrToIPv4(inaddr)
	if err != nil {
		return nil, newParseError("lookup address", inaddr)
	}

	Debug.Printf("[zonedb] Looking for address in local(&remote) database: '%s' (%s)", revIPv4, inaddr)

	zone.mx.RLock()
	now := zone.clock.Now()
	uniq := newUniqZoneRecords()
	for identName, nameset := range zone.idents {
		for _, ze := range nameset.getEntriesForIP(revIPv4) {
			// filter the entries with expired TTL
			// locally introduced entries are never expired: they always have TTL=0
			if ze.hasExpired(now) {
				Debug.Printf("[zonedb] '%s': expired entry '%s' ignored: removing", revIPv4, ze)
				nameset.deleteIP(revIPv4)
			} else {
				uniq.add(ze)
				if identName != defaultRemoteIdent {
					nameset.touchName(ze.Name(), now)
				}
			}
		}
	}
	zone.mx.RUnlock()

	if len(uniq) > 0 {
		Debug.Printf("[zonedb] '%s' resolved in local database", inaddr)
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

// TODO: for the sake of simplicity, we implement this mechanism with two channels: one for immediate
//       updates and another one for scheduled requests. We could use a heap of "next-time"s and a timer,
//       but that would require timers cancellations/updates/etc on insertions/removals/etc, and
//       it is probably not worth the trouble as we always use the same refresh period.
//       It could be useful if we move to a solution where we set update times from the responses TTLs,
//       but we currently use a fixed TTL (30secs), the same as the refresh period...
//       Anyway, maybe we will move to a gossip-based solution instead of doing this polling...

// Check if we must start updating a name and, in that case, trigger a immediate update
func (zone *zoneDb) startUpdatingName(name string) {
	if zone.refreshInterval > 0 {
		zone.mx.Lock()
		defer zone.mx.Unlock()

		// check if we should enqueue a refresh request for this name
		n := zone.getNamesSet(defaultRemoteIdent).getName(name, true)
		if n.lastRefreshTime.IsZero() {
			now := zone.clock.Now()
			n.lastRefreshTime = now

			Debug.Printf("[zonedb] Creating new immediate refresh request for '%s'", name)
			zone.refreshChan <- refreshRequest{name: name, time: now}
		}
	}
}

// A worker for updating the list of IPs we have for a name
func (zone *zoneDb) updater(num int) {
	defer zone.refreshWg.Done()

	Debug.Printf("[zonedb] Starting background updater #%d...", num)
	for {
		select {
		case <-zone.refreshCloseChan:
			Debug.Printf("[zonedb] Background updater #%d: interrupted while waiting for requests: exiting", num)
			return

		case request := <-zone.refreshChan:
			// if nobody has asked for this name for long time, just forget about it...
			// this will eventually garbage collect the `refreshChan` and all remote info in absence of activity
			if !zone.IsNameRelevant(request.name) || zone.IsNameExpired(request.name) {
				Debug.Printf("[zonedb] '%s' seem to be irrelevant now: removing any remote information", request.name)
				zone.mx.Lock()
				zone.getNamesSet(defaultRemoteIdent).deleteName(request.name)
				zone.mx.Unlock()
				continue
			}

			// perform the refresh for this name
			name := dns.Fqdn(request.name)
			Debug.Printf("[zonedb] Refreshing name '%s' with mDNS...", name)
			res, _ := zone.mdnsCli.InsistentLookupName(request.name)
			if res != nil && len(res) > 0 {
				numIps := len(res)
				zone.mx.Lock()
				now := zone.clock.Now()
				added, removed := zone.getNamesSet(defaultRemoteIdent).getName(name, true).updateIPs(res, now)
				zone.mx.Unlock()
				Debug.Printf("[zonedb] Obtained %d IPs for name '%s' with mDNS: %d added, %d removed",
					numIps, name, added, removed)

				// once the name has been updated, we insert the request (back) in the periodic requests channel
				now = zone.clock.Now()
				request.time = now.Add(zone.refreshInterval)
				Debug.Printf("[zonedb] Rescheduling update for '%s' in %.2f secs",
					request.name, zone.refreshInterval.Seconds())
				zone.refreshSchedChan <- request
			} else {
				Debug.Printf("[zonedb] nobody knows about '%s'... removing", name)
				zone.mx.Lock()
				zone.getNamesSet(defaultRemoteIdent).deleteName(request.name)
				zone.mx.Unlock()
			}
		}
	}
}

// The periodic updater
// Consume requests from the `refreshSchedChan`, where requests with increasing scheduling time are enqueued
// for refreshing names...
func (zone *zoneDb) periodicUpdater() {
	defer zone.refreshWg.Done()
	for {
		select {
		case <-zone.refreshCloseChan:
			Debug.Printf("[zonedb] Periodic updater: interrupted while waiting for requests: exiting")
			return

		case request := <-zone.refreshSchedChan:
			now := zone.clock.Now()
			// we can sleep until the update time has arrived, as requests are sorted by scheduled time (new request
			// in this channel will always be scheduled later than the last item in the channel), so we can
			// safely suspend this goroutine until then...
			if request.time.After(now) {
				ddiff := time.Duration(request.time.Sub(now).Nanoseconds())
				timer := zone.clock.Timer(ddiff)
				Debug.Printf("[zonedb] Periodic updater: new request for %s: sleeping for %.2f secs...",
					request.name, ddiff.Seconds())
				select {
				case <-zone.refreshCloseChan:
					Debug.Printf("[zonedb] Periodic updater: interrupted while sleeping: exiting")
					return
				case <-timer.C:
				}
			}

			// once the time has arrived, we insert the request in the immediate refresh channel
			zone.refreshChan <- request
		}
	}
}
