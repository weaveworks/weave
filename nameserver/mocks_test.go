package nameserver

import (
	"fmt"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
	. "github.com/weaveworks/weave/common"
)

const (
	testSocketTimeout = 100 // in millisecs
)

// Warn about some methods that some day should be implemented...
func notImplWarn() { Log.Warningf("Mocked method. Not implemented.") }

// A mocked Zone that always returns the same records
// * it does not send/receive any mDNS query
type mockedZoneWithRecords struct {
	sync.RWMutex

	records []ZoneRecord

	// Statistics
	NumLookupsName   int
	NumLookupsInaddr int
}

func newMockedZoneWithRecords(zr []ZoneRecord) *mockedZoneWithRecords {
	return &mockedZoneWithRecords{records: zr}
}
func (mz *mockedZoneWithRecords) Domain() string { return DefaultLocalDomain }
func (mz *mockedZoneWithRecords) LookupName(name string) ([]ZoneRecord, error) {
	Log.Debugf("[mocked zone]: LookupName: returning records %s", mz.records)
	mz.Lock()
	defer mz.Unlock()

	mz.NumLookupsName++
	var res []ZoneRecord
	for _, r := range mz.records {
		if r.Name() == name {
			res = append(res, r)
		}
	}
	return res, nil
}

func (mz *mockedZoneWithRecords) LookupInaddr(inaddr string) ([]ZoneRecord, error) {
	Log.Debugf("[mocked zone]: LookupInaddr: returning records %s", mz.records)
	mz.Lock()
	defer mz.Unlock()

	mz.NumLookupsInaddr++
	var res []ZoneRecord
	for _, r := range mz.records {
		revIP, err := raddrToIP(inaddr)
		if err != nil {
			return nil, newParseError("lookup address", inaddr)
		}
		if r.IP().Equal(revIP) {
			res = append(res, r)
		}
	}
	return res, nil
}

func (mz *mockedZoneWithRecords) DomainLookupName(name string) ([]ZoneRecord, error) {
	return mz.LookupName(name)
}
func (mz *mockedZoneWithRecords) DomainLookupInaddr(inaddr string) ([]ZoneRecord, error) {
	return mz.LookupInaddr(inaddr)
}

// the following methods are not currently needed...
func (mz *mockedZoneWithRecords) AddRecord(ident string, name string, ip net.IP) error {
	notImplWarn()
	return nil
}
func (mz *mockedZoneWithRecords) DeleteRecords(ident string, name string, ip net.IP) int {
	notImplWarn()
	return 0
}
func (mz *mockedZoneWithRecords) Status() string { notImplWarn(); return "nothing" }
func (mz *mockedZoneWithRecords) ObserveName(name string, observer ZoneRecordObserver) error {
	notImplWarn()
	return nil
}
func (mz *mockedZoneWithRecords) ObserveInaddr(inaddr string, observer ZoneRecordObserver) error {
	notImplWarn()
	return nil
}

//////////////////////////////////////////////////////////////////

// A mocked mDNS server
type mockedMDNSServer struct {
	zone Zone
}

func newMockedMDNSServer(z Zone) *mockedMDNSServer        { return &mockedMDNSServer{zone: z} }
func (ms *mockedMDNSServer) Start(_ *net.Interface) error { return nil }
func (ms *mockedMDNSServer) Stop() error                  { return nil }
func (ms *mockedMDNSServer) Zone() Zone                   { return ms.zone }

// a useful mix of NewMockedZone and NewMockedMDNSServer
func newMockedMDNSServerWithRecord(zr ZoneRecord) *mockedMDNSServer {
	return newMockedMDNSServer(newMockedZoneWithRecords([]ZoneRecord{zr}))
}
func newMockedMDNSServerWithRecords(zrs []ZoneRecord) *mockedMDNSServer {
	return newMockedMDNSServer(newMockedZoneWithRecords(zrs))
}

//////////////////////////////////////////////////////////////////

// A mocked mDNS client.
// This mock asks a group of (potentially mocked) mDNS servers
type mockedMDNSClient struct {
	sync.RWMutex

	servers map[ZoneMDNSServer]ZoneMDNSServer

	// Statistics
	NumLookupsName      int
	NumLookupsInaddr    int
	NumInsLookupsName   int
	NumInsLookupsInaddr int
}

func newMockedMDNSClient(srvrs []*mockedMDNSServer) *mockedMDNSClient {
	r := mockedMDNSClient{
		servers: make(map[ZoneMDNSServer]ZoneMDNSServer),
	}
	if srvrs != nil {
		for _, server := range srvrs {
			r.servers[server] = server
		}
	}
	return &r
}

func (mc *mockedMDNSClient) Start(ifi *net.Interface) error { return nil }
func (mc *mockedMDNSClient) Stop() error                    { return nil }
func (mc *mockedMDNSClient) Domain() string                 { return DefaultLocalDomain }
func (mc *mockedMDNSClient) LookupName(name string) ([]ZoneRecord, error) {
	mc.Lock()
	defer mc.Unlock()

	// get a random result
	mc.NumLookupsName++
	for _, server := range mc.servers {
		r, err := server.Zone().LookupName(name)
		if err == nil {
			return r, nil // return the first answer
		}
	}
	return nil, LookupError(name)
}

func (mc *mockedMDNSClient) LookupInaddr(inaddr string) ([]ZoneRecord, error) {
	mc.Lock()
	defer mc.Unlock()

	// get a random result
	mc.NumLookupsInaddr++
	for _, server := range mc.servers {
		r, err := server.Zone().LookupInaddr(inaddr)
		if err == nil {
			return r, nil // return the first answer
		}
	}
	return nil, LookupError(inaddr)
}

func (mc *mockedMDNSClient) InsistentLookupName(name string) ([]ZoneRecord, error) {
	mc.Lock()
	defer mc.Unlock()

	var res []ZoneRecord
	mc.NumInsLookupsName++
	for _, server := range mc.servers {
		r, err := server.Zone().LookupName(name)
		if err == nil {
			res = append(res, r...)
		}
	}
	if len(res) == 0 {
		return nil, LookupError(name)
	}
	return res, nil
}

func (mc *mockedMDNSClient) InsistentLookupInaddr(inaddr string) ([]ZoneRecord, error) {
	mc.Lock()
	defer mc.Unlock()

	var res []ZoneRecord
	mc.NumInsLookupsInaddr++
	for _, server := range mc.servers {
		r, err := server.Zone().LookupInaddr(inaddr)
		if err == nil {
			res = append(res, r...)
		}
	}
	if len(res) == 0 {
		return nil, LookupError(inaddr)
	}
	return res, nil
}

func (mc *mockedMDNSClient) AddServer(srv ZoneMDNSServer) {
	mc.Lock()
	defer mc.Unlock()
	mc.servers[srv] = srv
}

func (mc *mockedMDNSClient) RemoveServer(srv ZoneMDNSServer) {
	mc.Lock()
	defer mc.Unlock()
	delete(mc.servers, srv)
}

//////////////////////////////////////////////////////////////////

type zbmEntry struct {
	Server *mockedMDNSServer
	Client *mockedMDNSClient
	Zone   *ZoneDb
}
type zoneDbsWithMockedMDns []*zbmEntry

// Creates a group of (real) zone databases linked through mocked mDNS servers and clients
// This effectively creates a cluster of WeaveDNS peers
func newZoneDbsWithMockedMDns(num int, config ZoneConfig) zoneDbsWithMockedMDns {
	res := make(zoneDbsWithMockedMDns, num)

	for i := 0; i < num; i++ {
		res[i] = new(zbmEntry)

		Log.Debugf("[test] Creating mocked mDNS server #%d", i)
		res[i].Server = newMockedMDNSServer(nil)

		Log.Debugf("[test] Creating mocked mDNS client #%d", i)
		res[i].Client = newMockedMDNSClient([]*mockedMDNSServer{})

		cfg := config
		cfg.MDNSClient = res[i].Client
		cfg.MDNSServer = res[i].Server

		Log.Debugf("[test] Creating ZoneDb #%d", i)
		res[i].Zone, _ = NewZoneDb(cfg)

		// link the mDNS server to its zone
		// ZoneDbs are connected to mDNS in two directions: they use their mDNS
		// clients for asking other peers about names/IPs, and export their records
		// through a mDNS server.
		res[i].Server.zone = res[i].Zone
	}

	// link all the clients to all the servers
	for i := 0; i < num; i++ {
		for j := 0; j < num; j++ {
			// with this check, the local client will not ask to the local server
			// this is not what we currently do, but can be convenient in some cases
			// for finding some bugs...
			if i != j {
				Log.Debugf("[test] Linking mocked mDNS client #%d to server #%d", i, j)
				res[i].Client.AddServer(res[j].Server)
			}
		}
	}

	return res
}

func (zbs zoneDbsWithMockedMDns) Start() {
	for _, entry := range zbs {
		entry.Zone.Start()
	}
}

func (zbs zoneDbsWithMockedMDns) Stop() {
	for _, entry := range zbs {
		entry.Zone.Stop()
	}
}

func (zbs zoneDbsWithMockedMDns) Flush() {
	for _, entry := range zbs {
		entry.Zone.refreshScheds.Flush()
	}
}

//////////////////////////////////////////////////////////////////

// A mocked cache where we never find a single thing... ;)
type mockedCache struct {
	NumGets     int
	NumPuts     int
	NumRemovals int
}

func newMockedCache() *mockedCache { return &mockedCache{} }

func (c *mockedCache) Get(request *dns.Msg, maxLen int) (reply *dns.Msg, err error) {
	c.NumGets++
	return nil, nil
}
func (c *mockedCache) Put(request *dns.Msg, reply *dns.Msg, ttl int, flags uint8) int {
	c.NumPuts++
	return 0
}
func (c *mockedCache) Remove(question *dns.Question) { c.NumRemovals++ }
func (c *mockedCache) Purge()                        {}
func (c *mockedCache) Clear()                        {}
func (c *mockedCache) Len() int                      { return 0 }
func (c *mockedCache) Capacity() int                 { return DefaultCacheLen }

//////////////////////////////////////////////////////////////////

// A mocked fallback server
type mockedFallback struct {
	CliConfig *dns.ClientConfig
	Addr      string
	Port      int

	udpSrv *dns.Server
	tcpSrv *dns.Server
}

// Create a mocked fallback server
// You can use the server's `CliConfig` as the `UpstreamCfg` of a `DNSServer`...
func newMockedFallback(udpH dns.HandlerFunc, tcpH dns.HandlerFunc) (*mockedFallback, error) {
	udpSrv, fallbackAddr, err := runLocalUDPServer("127.0.0.1:0", udpH)
	if err != nil {
		return nil, err
	}
	_, fallbackPort, err := net.SplitHostPort(fallbackAddr)
	if err != nil {
		return nil, err
	}

	fallbackPortI, _ := strconv.Atoi(fallbackPort)

	res := mockedFallback{
		udpSrv: udpSrv,
		Addr:   fallbackAddr,
		Port:   fallbackPortI,
		CliConfig: &dns.ClientConfig{
			Servers: []string{"127.0.0.1"},
			Port:    fallbackPort,
		},
	}

	if tcpH != nil {
		fallbackTCPAddr := fmt.Sprintf("127.0.0.1:%s", fallbackPort)
		res.tcpSrv, _, err = runLocalTCPServer(fallbackTCPAddr, tcpH)
		if err != nil {
			return nil, err
		}
	}

	return &res, nil
}

// Start the fallback server
func (mf *mockedFallback) Start() error {
	return nil
}

// Stop the fallback server
func (mf *mockedFallback) Stop() error {
	if mf.tcpSrv != nil {
		mf.udpSrv.Shutdown()
		mf.udpSrv = nil
	}
	if mf.tcpSrv != nil {
		mf.tcpSrv.Shutdown()
		mf.udpSrv = nil
	}
	return nil
}

// Run a UDP fallback server
func runLocalUDPServer(laddr string, handler dns.HandlerFunc) (*dns.Server, string, error) {
	Log.Debugf("[mocked fallback] Starting fallback UDP server at %s", laddr)
	pc, err := net.ListenPacket("udp", laddr)
	if err != nil {
		return nil, "", err
	}
	server := &dns.Server{PacketConn: pc, Handler: handler, ReadTimeout: testSocketTimeout * time.Millisecond}

	go func() {
		server.ActivateAndServe()
		pc.Close()
	}()

	Log.Debugf("[mocked fallback] Fallback UDP server listening at %s", pc.LocalAddr())
	return server, pc.LocalAddr().String(), nil
}

// Run a TCP fallback server
func runLocalTCPServer(laddr string, handler dns.HandlerFunc) (*dns.Server, string, error) {
	Log.Debugf("[mocked fallback] Starting fallback TCP server at %s", laddr)
	laddrTCP, err := net.ResolveTCPAddr("tcp", laddr)
	if err != nil {
		return nil, "", err
	}

	l, err := net.ListenTCP("tcp", laddrTCP)
	if err != nil {
		return nil, "", err
	}
	server := &dns.Server{Listener: l, Handler: handler, ReadTimeout: testSocketTimeout * time.Millisecond}

	go func() {
		server.ActivateAndServe()
		l.Close()
	}()

	Log.Debugf("[mocked fallback] Fallback TCP server listening at %s", l.Addr().String())
	return server, l.Addr().String(), nil
}

//////////////////////////////////////////////////////////////////

type mockedClock struct {
	*clock.Mock
}

func newMockedClock() *mockedClock {
	return &mockedClock{clock.NewMock()}
}

// Note: moving the clock forward does not take into account the data that could
//       be waiting in channels. So your code should not depend on the channels
//       for creating new timers. Otherwise, time travelling will not be reliable...
func (clk *mockedClock) Forward(secs int) {
	Log.Debugf(">>>>>>> Moving clock forward %d seconds - Time traveling >>>>>>>", secs)
	clk.Add(time.Duration(secs) * time.Second)
	Log.Debugf("<<<<<<< Time travel finished! We are at %s <<<<<<<", clk.Now())
}

//////////////////////////////////////////////////////////////////

// Perform a DNS query and assert the reply code, number or answers, etc
func assertExchange(t *testing.T, z string, ty uint16, port int, minAnswers int, maxAnswers int, expErr int) (*dns.Msg, *dns.Msg) {
	require.NotEqual(t, 0, port, "invalid DNS server port")

	c := &dns.Client{
		UDPSize: testUDPBufSize,
	}

	m := new(dns.Msg)
	m.RecursionDesired = true
	m.SetQuestion(z, ty)
	m.SetEdns0(testUDPBufSize, false) // we don't want to play with truncation here...

	lstAddr := fmt.Sprintf("127.0.0.1:%d", port)
	r, _, err := c.Exchange(m, lstAddr)
	t.Logf("Response from '%s':\n%+v\n", lstAddr, r)
	if err != nil {
		t.Errorf("Error when querying DNS server at %s: %s", lstAddr, err)
	}
	require.NoError(t, err)
	if minAnswers == 0 && maxAnswers == 0 {
		require.Equal(t, expErr, r.Rcode, "DNS response code")
	} else {
		require.Equal(t, dns.RcodeSuccess, r.Rcode, "DNS response code")
	}
	answers := len(r.Answer)
	if minAnswers >= 0 && answers < minAnswers {
		require.FailNow(t, fmt.Sprintf("Number of answers >= %d", minAnswers))
	}
	if maxAnswers >= 0 && answers > maxAnswers {
		require.FailNow(t, fmt.Sprintf("Number of answers <= %d", maxAnswers))
	}
	return m, r
}

// Assert that we have a response in the cache for a query `q`
func assertInCache(t *testing.T, cache ZoneCache, q *dns.Msg, desc string) {
	r, err := cache.Get(q, maxUDPSize)
	require.NoError(t, err)
	require.NotNil(t, r, fmt.Sprintf("value in the cache: %s", desc))
}

// Assert that we have a response in the cache for a query `q`
func assertNotLocalInCache(t *testing.T, cache ZoneCache, q *dns.Msg, desc string) {
	r, err := cache.Get(q, maxUDPSize)
	if !(r == nil && err == errNoLocalReplies) {
		t.Fatalf("Cache does not return a noLocalReplies error for query %s", q)
	}
}

// Assert that we do not have a response in the cache for a query `q`
func assertNotInCache(t *testing.T, cache ZoneCache, q *dns.Msg, desc string) {
	r, err := cache.Get(q, maxUDPSize)
	require.NoError(t, err)
	require.Nil(t, r, fmt.Sprintf("value in the cache: %s\n%s", desc, r))
}
