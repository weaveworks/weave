package nameserver

import (
	"bytes"
	"fmt"
	"github.com/benbjohnson/clock"
	"github.com/miekg/dns"
	. "github.com/weaveworks/weave/common"
	"net"
	"sync"
	"time"
)

const (
	RDNSDomain = "in-addr.arpa."
)

const (
	DefaultLocalDomain     = "weave.local." // The default name used for the local domain
	DefaultRefreshInterval = int(localTTL)  // Period for background updates with mDNS
	DefaultRelevantTime    = 60             // When to forget info about remote info if nobody asks...
	DefaultNumUpdaters     = 4              // Default number of background updaters

	defaultRefreshMailbox = 1000           // Number of on-the-fly background queries
	defaultRemoteIdent    = "weave:remote" // Ident used for info obtained from mDNS
)

// +1 to also exclude a dot
var rdnsDomainLen = len(RDNSDomain) + 1

type LookupError string

func (ops LookupError) Error() string {
	return "Unable to find " + string(ops)
}

type DuplicateError struct {
}

func (dup DuplicateError) Error() string {
	return "Tried to add a duplicate entry"
}

type Zone interface {
	ZoneObservable
	ZoneLookup
	// The domain where we operate (eg, "weave.local.")
	Domain() string
	// Add a record in the local database
	AddRecord(ident string, name string, ip net.IP) error
	// Delete a record in the local database
	DeleteRecord(ident string, ip net.IP) error
	// Delete all records for an ident in the local database
	DeleteRecordsFor(ident string) error
	// Lookup for a name in the whole domain
	DomainLookupName(name string) ([]ZoneRecord, error)
	// Lookup for an address in the whole domain
	DomainLookupInaddr(inaddr string) ([]ZoneRecord, error)
	// Return a status string
	Status() string
}

///////////////////////////////////////////////////////////////////
//
// Zone database overview:
//
//           idents            names             entry
//         *-------*          *------*         *------*
// zone -> | ident |  --*-->  | name |  --*->  | IPv4 |
//         *-------*    |     *------*    |    *------*
//         | ident |    *-->  | name |    *->  | IPv4 |
//         *-------*          *------*         *------*
//            ...               ...
//
// The zone database keeps an ident per container, but also a special
// ident for names/IPs obtained from remote peers.
//
// Each name can have multiple IPs (so far, we only consider IPv4s), and
// each IP is stored in a `recordEntry` in the database. Entries store
// some additional information like priority and weight of the IP (for
// example, for future use with SRV records)
//

// An entry in the zone database
type recordEntry struct {
	Record

	observers []ZoneRecordObserver // the observers for this record
	insTime   time.Time            // time when this record was inserted
}

func newRecordEntry(zr ZoneRecord, now time.Time) *recordEntry {
	return &recordEntry{
		Record: Record{
			name:     zr.Name(),
			ip:       zr.IP(),
			priority: zr.Priority(),
			weight:   zr.Weight(),
			ttl:      zr.TTL(),
		},
		insTime:   now,
		observers: make([]ZoneRecordObserver, 0),
	}
}

func (i *recordEntry) addIPObserver(zro ZoneRecordObserver) {
	i.observers = append(i.observers, zro)
}

// Notify all the IP observers and then remove them all
// IP address observers are notified when
// - the IP is removed
// - the name is removed
func (re *recordEntry) notifyIPObservers() {
	numObservers := len(re.observers)
	if numObservers > 0 {
		Debug.Printf("[zonedb] Notifying %d observers of '%s'", numObservers, re.ip)
		for _, observer := range re.observers {
			observer()
		}
		re.observers = make([]ZoneRecordObserver, 0)
	}
}

func (re *recordEntry) TTL() int {
	// if we never set the TTL (eg, when using AddRecord()), return the standard value...
	if re.ttl == 0 {
		return int(localTTL)
	}
	return re.ttl
}

// Check if the info in this record is still valid according to the TTL
func (re *recordEntry) hasExpired(now time.Time) bool {
	return re.ttl > 0 && now.Sub(re.insTime).Seconds() > float64(re.ttl)
}

func (re *recordEntry) String() string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%s", re.Record)
	lobs := len(re.observers)
	if lobs > 0 {
		fmt.Fprintf(&buf, "/OBS:%d", lobs)
	}
	return buf.String()
}

///////////////////////////////////////////////////////////////////

// a name, with group of records by IPv4
type name struct {
	name            string
	ipv4            map[IPv4]*recordEntry // all the IPv4 records for this name
	observers       []ZoneRecordObserver  // the observers for this name
	lastRefreshTime time.Time             // last time this name was updated by broadcasting a query
	lastAccessTime  time.Time             // last time a Lookup* used info from this name
}

func newName(n string) *name {
	return &name{
		name:      n,
		ipv4:      make(map[IPv4]*recordEntry),
		observers: make([]ZoneRecordObserver, 0),
	}
}
func (n *name) len() int             { return len(n.ipv4) }
func (n *name) empty() bool          { return n.len() == 0 }
func (n *name) hasIP(ip net.IP) bool { return n.hasIPv4(ipToIPv4(ip)) }
func (n *name) hasIPv4(ip IPv4) bool { _, found := n.ipv4[ip]; return found }

// Get all the entries for this name
func (n *name) getAllEntries() (res []*recordEntry) {
	for _, entry := range n.ipv4 {
		res = append(res, entry)
	}
	return
}

// Get the entry for an IP in this name
func (n *name) getEntryForIP(ip IPv4) (res *recordEntry) {
	if entry, found := n.ipv4[ip]; found {
		res = entry
	}
	return
}

// Add a new IPv4 to this name
func (n *name) addIP(zr ZoneRecord, now time.Time) (*recordEntry, error) {
	ne := newRecordEntry(zr, now)
	ipv4 := ipToIPv4(zr.IP())
	n.ipv4[ipv4] = ne
	n.notifyNameObservers()
	return ne, nil
}

// Delete an IP for this name
func (n *name) deleteIP(ip IPv4) bool {
	if ipRecord, found := n.ipv4[ip]; found {
		ipRecord.notifyIPObservers()
		n.notifyNameObservers()
		delete(n.ipv4, ip)
		return true
	}
	return false
}

// Update the list of IPs for this name, adding new ones and removing old IPs...
func (n *name) updateIPs(records []ZoneRecord, now time.Time) (added int, removed int) {
	ipsMap := make(map[IPv4]bool)

	// add the new IPs
	for _, record := range records {
		ipv4 := ipToIPv4(record.IP())
		ipsMap[ipv4] = true
		if !n.hasIPv4(ipv4) {
			n.addIP(record, now)
			added++
		}
	}

	// remove the old IPs
	for ipv4, ipRecord := range n.ipv4 {
		if _, isNew := ipsMap[ipv4]; !isNew {
			ipRecord.notifyIPObservers() // we must notify the observers before removing the IP record...
			delete(n.ipv4, ipv4)
			removed++
		}
	}

	// notify observers and update times
	if added > 0 || removed > 0 {
		n.lastRefreshTime = now
		n.notifyNameObservers()
	}
	return
}

// Add a observer for this name
func (n *name) addNameObserver(observer ZoneRecordObserver) {
	n.observers = append(n.observers, observer)
}

// Notify and flush all the observers
// Name observers are notified when
// - an IP is added
// - an IP is removed
// - the name is removed
func (n *name) notifyNameObservers() {
	numObservers := len(n.observers)
	if numObservers > 0 {
		Debug.Printf("[zonedb] Notifying %d observers of '%s'", numObservers, n.name)
		for _, observer := range n.observers {
			observer()
		}
		n.observers = make([]ZoneRecordObserver, 0)
	}
}

// Convert a set of IP records to a comma-separated string
func (n *name) String() string {
	var buf bytes.Buffer
	for _, ip := range n.ipv4 {
		fmt.Fprintf(&buf, "%s, ", ip)
	}
	l := buf.Len()
	if l > 2 {
		buf.Truncate(l - 2)
	}
	return buf.String()
}

///////////////////////////////////////////////////////////////////

// all the names in a ident
type namesSet struct {
	names map[string]*name
}

func newNamesSet() *namesSet     { return &namesSet{names: make(map[string]*name)} }
func (ns *namesSet) len() int    { return len(ns.names) }
func (ns *namesSet) empty() bool { return ns.len() == 0 }
func (ns *namesSet) hasName(name string) bool {
	if n, found := ns.names[name]; !found {
		return false
	} else {
		// the name must have some valid IPs in order to be considered as "existent"
		return len(n.ipv4) > 0
	}
}

// Get the zone entries for a name
// If the name does not exist and `create` is true, a new name will be created
func (ns *namesSet) getName(name string, create bool) (n *name) {
	if n, found := ns.names[name]; !found && create {
		newName := newName(name)
		ns.names[name] = newName
		return newName
	} else {
		return n
	}
}

// Get all the entries for a name
func (ns *namesSet) getEntriesForName(name string) (res []*recordEntry) {
	n := ns.getName(name, false)
	if n != nil {
		res = n.getAllEntries()
	}
	return
}

// Get all the entries for an IP, for all names
func (ns *namesSet) getEntriesForIP(ip IPv4) (res []*recordEntry) {
	for _, name := range ns.names {
		entry := name.getEntryForIP(ip)
		if entry != nil {
			res = append(res, entry)
		}
	}
	return
}

// Add a new IPv4 to a name
func (ns *namesSet) addIPToName(zr ZoneRecord, now time.Time) (*recordEntry, error) {
	n := ns.getName(zr.Name(), true)
	ipv4 := ipToIPv4(zr.IP())
	if n.hasIPv4(ipv4) {
		return n.getEntryForIP(ipv4), DuplicateError{}
	}
	ns.touchName(zr.Name(), now)
	return n.addIP(zr, now)
}

// Delete a name in the names set
func (ns *namesSet) deleteName(n string) error {
	if name, found := ns.names[n]; found {
		for _, ipRecord := range name.getAllEntries() {
			ipRecord.notifyIPObservers()
		}
		name.notifyNameObservers()
		delete(ns.names, n)
		return nil
	} else {
		return LookupError(fmt.Sprintf("%+v", name))
	}
}

// Delete an IPv4 from all names
func (ns *namesSet) deleteIP(ip IPv4) error {
	cnt := 0
	for nkey, n := range ns.names {
		if n.deleteIP(ip) {
			cnt++
		}
		if n.empty() {
			delete(ns.names, nkey)
		}
	}

	// we must return an error if no records have been found and deleted
	if cnt == 0 {
		return LookupError(fmt.Sprintf("%+v", ip))
	}
	return nil
}

// Get the name query time
func (ns *namesSet) getNameLastAccess(n string) time.Time {
	if name, found := ns.names[n]; found {
		return name.lastAccessTime
	}
	return time.Time{}
}

// Touch the last access time for a name
// The access time is saved only for locally-introduced records (otherwise it could
// be lost when names are irrelevant...)
func (ns *namesSet) touchName(name string, now time.Time) {
	Debug.Printf("[zonedb] Touching name %s", name)
	n := ns.getName(name, false)
	if n != nil {
		n.lastAccessTime = now
	}
}

func (ns *namesSet) String() string {
	var buf bytes.Buffer
	for _, name := range ns.names {
		fmt.Fprintf(&buf, "%s, ", name)
	}
	l := buf.Len()
	if l > 2 {
		buf.Truncate(l - 2)
	}
	return buf.String()
}

///////////////////////////////////////////////////////////////////

// Names sets, by ident name
type identRecordsSet map[string]*namesSet

func (irs identRecordsSet) String() string {
	var buf bytes.Buffer
	for ident, nameset := range irs {
		fmt.Fprintf(&buf, "%.12s: %s\n", ident, nameset)
	}
	return buf.String()
}

///////////////////////////////////////////////////////////////////

// Invariant on updateRequests: they are in ascending order of time.
type refreshRequest struct {
	name string
	time time.Time
}

type ZoneConfig struct {
	// The local domain
	Domain string
	// The interface where mDNS operates
	Iface *net.Interface
	// Refresh interval for local names in the zone database (disabled by default) (in seconds)
	RefreshInterval int
	// Number of background updaters for names
	RefreshWorkers int
	// Forget about remote info if nobody asks for it in this time (in seconds)
	RelevantTime int
	// Max pending refresh requests
	MaxRefreshRequests int
	// Force a specific mDNS client
	MDNSClient ZoneMDNSClient
	// Force a specific mDNS server
	MDNSServer ZoneMDNSServer
	// For a specific clock provider
	Clock clock.Clock
}

// Zone database
// The zone database contains the locally known information about the zone.
// The zone database uses some other mechanisms (eg, mDNS) for finding unknown
// information or for keeping current information up to date.
type zoneDb struct {
	mx     sync.RWMutex
	idents identRecordsSet

	mdnsCli       ZoneMDNSClient
	mdnsSrv       ZoneMDNSServer
	iface         *net.Interface
	domain        string        // the local domain
	relevantLimit time.Duration // if no one asks for something in this time, it is not relevant a anymore...
	clock         clock.Clock

	refreshChan      chan refreshRequest // channel for enqueing requests for updating names
	refreshSchedChan chan refreshRequest // channel for enqueing requests for updating names
	refreshCloseChan chan bool
	refreshWg        sync.WaitGroup
	refreshInterval  time.Duration
	refreshWorkers   int
}

// Create a new zone database
func NewZoneDb(config ZoneConfig) (zone *zoneDb, err error) {
	maxRefreshRequests := defaultRefreshMailbox
	if config.MaxRefreshRequests > 0 {
		maxRefreshRequests = config.MaxRefreshRequests
	}

	zone = &zoneDb{
		domain:           config.Domain,
		idents:           make(identRecordsSet),
		mdnsCli:          config.MDNSClient,
		mdnsSrv:          config.MDNSServer,
		iface:            config.Iface,
		clock:            config.Clock,
		relevantLimit:    time.Duration(DefaultRelevantTime) * time.Second,
		refreshChan:      make(chan refreshRequest, maxRefreshRequests),
		refreshSchedChan: make(chan refreshRequest, maxRefreshRequests),
		refreshCloseChan: make(chan bool),
		refreshWorkers:   DefaultNumUpdaters,
	}

	// fix the default configuration parameters
	if zone.clock == nil {
		zone.clock = clock.New()
	}
	if len(zone.domain) == 0 {
		zone.domain = DefaultLocalDomain
	}
	if config.RefreshInterval > 0 {
		zone.refreshInterval = time.Duration(config.RefreshInterval) * time.Second
	}
	if config.RelevantTime > 0 {
		zone.relevantLimit = time.Duration(config.RelevantTime) * time.Second
	}
	if config.RefreshWorkers > 0 {
		zone.refreshWorkers = config.RefreshWorkers
	}

	// Create the mDNS client and server
	if zone.mdnsCli == nil {
		if zone.mdnsCli, err = NewMDNSClient(); err != nil {
			return
		}
	}
	if zone.mdnsSrv == nil {
		if zone.mdnsSrv, err = NewMDNSServer(zone, false); err != nil {
			return
		}
	}

	return
}

// Start the zone database
func (zone *zoneDb) Start() (err error) {
	if zone.iface != nil {
		Info.Printf("[zonedb] Using mDNS on %+v", zone.iface)
	} else {
		Info.Printf("[zonedb] Using mDNS on all interfaces")
	}

	if err = zone.mdnsCli.Start(zone.iface); err != nil {
		return
	}
	if err = zone.mdnsSrv.Start(zone.iface); err != nil {
		return
	}

	// Start some name refreshers...
	if zone.refreshInterval > 0 {
		Info.Printf("[zonedb] Starting %d background name updaters", zone.refreshWorkers)
		for i := 0; i < zone.refreshWorkers; i++ {
			zone.refreshWg.Add(1)
			go zone.updater(i)
		}

		zone.refreshWg.Add(1)
		go zone.periodicUpdater()
	} else {
		// TODO: the refresher also garbage collects irrelevant remote info... we should have an alternative mechanism
		//       when we don't use refreshers.
		Warning.Printf("[zonedb] Remote info will not be garbage collected")
	}

	return
}

// Perform a graceful shutdown of the zone database
func (zone *zoneDb) Stop() error {
	if zone.refreshInterval > 0 {
		Debug.Printf("[zonedb] Closing background updaters...")
		close(zone.refreshCloseChan)
		zone.refreshWg.Wait()
	}

	Debug.Printf("[zonedb] Exiting mDNS client and server...")
	zone.mdnsCli.Stop()
	zone.mdnsSrv.Stop()
	return nil
}

// Obtain the domain where this database keeps information for
func (zone *zoneDb) Domain() string {
	return zone.domain
}

// Return the status string
func (zone *zoneDb) Status() string {
	var buf bytes.Buffer
	fmt.Fprintln(&buf, "Local domain", zone.domain)
	fmt.Fprintln(&buf, "Interface", zone.iface)
	fmt.Fprintf(&buf, "Zone database:\n%s", zone)
	return buf.String()
}

// Add a record with `name` pointing to `ip`
func (zone *zoneDb) AddRecord(ident string, name string, ip net.IP) (err error) {
	zone.mx.Lock()
	defer zone.mx.Unlock()
	Debug.Printf("[zonedb] Adding record: '%s'/'%s'[%s]", ident, name, ip)
	record := Record{dns.Fqdn(name), ip, 0, 0, 0}
	_, err = zone.getNamesSet(ident).addIPToName(record, zone.clock.Now())
	return
}

// Delete all records for an ident
func (zone *zoneDb) DeleteRecordsFor(ident string) (err error) {
	zone.mx.Lock()
	defer zone.mx.Unlock()
	Debug.Printf("[zonedb] Deleting records for ident '%s'", ident)
	zone.deleteIdent(ident)
	return
}

// Delete an IP in a ident
func (zone *zoneDb) DeleteRecord(ident string, ip net.IP) (err error) {
	zone.mx.Lock()
	defer zone.mx.Unlock()

	Debug.Printf("[zonedb] Deleting record '%s'/[%s]", ident, ip)
	if _, found := zone.idents[ident]; found {
		err = zone.idents[ident].deleteIP(ipToIPv4(ip))
		if zone.idents[ident].empty() {
			zone.deleteIdent(ident)
		}
	} else {
		err = LookupError(ident)
	}
	return
}

// Observe a name.
// The name must have at least one valid IP address.
// Name observers are notified when
// - an IP is added
// - an IP is removed
// - the name is removed
// The observer will be invoked on any change/removal that affects this name.
// Each observer will be invoked at least once (but possibly more). After that, they will be removed.
// The observer should not try to lock the ZoneDB (you will get a deadlock)
func (zone *zoneDb) ObserveName(name string, observer ZoneRecordObserver) (err error) {
	zone.mx.Lock()
	defer zone.mx.Unlock()

	cnt := 0
	name = dns.Fqdn(name)
	for _, nameset := range zone.idents {
		if n := nameset.getName(name, false); n != nil {
			n.addNameObserver(observer)
			cnt += 1
		}
	}
	if cnt == 0 {
		err = LookupError(fmt.Sprintf("%s", name))
	}
	return
}

// Observe a IP address.
// The IP address must be exit in the database.
// IP address observers are notified when
// - the IP is removed
// - the name is removed
// The observer will be invoked on any change/removal that affects this IP.
// Each observer will be invoked at least once (but possibly more). After that, they will be removed.
// The observer should not try to lock the ZoneDB (you will get a deadlock)
func (zone *zoneDb) ObserveInaddr(inaddr string, observer ZoneRecordObserver) (err error) {
	zone.mx.Lock()
	defer zone.mx.Unlock()

	cnt := 0
	revIP, err := raddrToIPv4(inaddr)
	if err != nil {
		return newParseError("lookup address", inaddr)
	}
	for _, nameset := range zone.idents {
		for _, entry := range nameset.getEntriesForIP(revIP) {
			entry.addIPObserver(observer)
			cnt += 1
		}
	}
	if cnt == 0 {
		err = LookupError(fmt.Sprintf("%+v/%+v", inaddr, revIP))
	}
	return
}

// Get the string representation of a zone
func (zone *zoneDb) String() string {
	zone.mx.RLock()
	defer zone.mx.RUnlock()
	return zone.idents.String()
}

// Notify that a container has died
func (zone *zoneDb) ContainerDied(ident string) error {
	Info.Printf("[zonedb] Container %s down. Removing records", ident)
	return zone.DeleteRecordsFor(ident)
}

// Return true if a remote name is still relevant
func (zone *zoneDb) IsNameRelevant(n string) bool {
	zone.mx.Lock()
	defer zone.mx.Unlock()

	lastAccess := time.Time{}
	for _, nameset := range zone.idents {
		if t := nameset.getNameLastAccess(n); !t.IsZero() {
			if lastAccess.IsZero() || t.After(lastAccess) {
				lastAccess = t
			}
		}
	}

	return !lastAccess.IsZero() && zone.clock.Now().Sub(lastAccess) < zone.relevantLimit
}

// Returns true if the info we have for a remote name has expired. Local names are never expired
// Returns false if we do not have info for this name from remote peers.
func (zone *zoneDb) IsNameExpired(n string) bool {
	zone.mx.Lock()
	defer zone.mx.Unlock()

	now := zone.clock.Now()
	remoteNamesSet := zone.getNamesSet(defaultRemoteIdent)
	entries := remoteNamesSet.getEntriesForName(n)
	if entries != nil {
		for _, entry := range entries {
			if entry.hasExpired(now) {
				return true
			}
		}
	}
	return false
}

func (zone *zoneDb) HasNameLocalInfo(n string) bool {
	zone.mx.Lock()
	defer zone.mx.Unlock()

	remoteNamesSet := zone.getNamesSet(defaultRemoteIdent)
	for _, nameset := range zone.idents {
		if nameset != remoteNamesSet {
			if nameset.hasName(n) {
				return true
			}
		}
	}
	return false
}

// Return true if the we have obtained some information for a name from remote peers
// We can have both local (eg, introduced through the HTTP API) and remote (eg, mDNS)
// information for a name. This method only checks if there is remote information...
func (zone *zoneDb) HasNameRemoteInfo(n string) bool {
	zone.mx.Lock()
	defer zone.mx.Unlock()

	remoteNamesSet := zone.getNamesSet(defaultRemoteIdent)
	if remoteNamesSet != nil {
		return remoteNamesSet.hasName(n)
	}
	return false
}

// Get or create a names set
func (zone *zoneDb) getNamesSet(ident string) *namesSet {
	ns, found := zone.idents[ident]
	if !found {
		ns = newNamesSet()
		zone.idents[ident] = ns
	}
	return ns
}

// Delete a ident
func (zone *zoneDb) deleteIdent(ident string) {
	Info.Printf("[zonedb] Removing everything for ident '%s'", ident)
	if ns, found := zone.idents[ident]; found {
		for _, n := range ns.names {
			for _, entry := range n.getAllEntries() {
				entry.notifyIPObservers()
			}
			n.notifyNameObservers()
		}
		delete(zone.idents, ident)
	}
}
