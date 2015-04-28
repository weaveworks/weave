package nameserver

import (
	"bytes"
	"fmt"
	"github.com/miekg/dns"
	. "github.com/weaveworks/weave/common"
	"net"
	"sync"
)

const (
	DefaultLocalDomain   = "weave.local."     // The default name used for the local domain
	DefaultServerPort    = 53                 // The default server port
	DefaultCLICfgFile    = "/etc/resolv.conf" // default "resolv.conf" file to try to load
	DefaultUDPBuflen     = 4096               // bigger than the default 512
	DefaultCacheLen      = 8192               // default cache capacity
	DefaultResolvWorkers = 8                  // default number of resolution workers
	DefaultTimeout       = 5                  // default timeout for DNS resolutions
)

type DNSServerConfig struct {
	// (Optional) client config file for resolving upstream servers
	UpstreamCfgFile string
	// (Optional) DNS client config for the fallback server(s)
	UpstreamCfg *dns.ClientConfig
	// (Optional) port number (for TCP and UDP)
	Port int
	// (Optional) local domain (ie, "weave.local.")
	LocalDomain string
	// (Optional) cache size
	CacheLen int
	// (Optional) timeout for DNS queries
	Timeout int
	// (Optional) UDP buffer length
	UDPBufLen int
}

type dnsProtocol uint8

const (
	protUDP dnsProtocol = iota // UDP protocol
	protTCP dnsProtocol = iota // TCP protocol
)

func (proto dnsProtocol) String() string {
	switch proto {
	case protUDP:
		return "UDP"
	case protTCP:
		return "TCP"
	}
	return "unknown"
}

// get a new dns.Client for a protocol
func (proto dnsProtocol) GetNewClient(bufsize int) *dns.Client {
	switch proto {
	case protTCP:
		return &dns.Client{Net: "tcp"}
	case protUDP:
		return &dns.Client{Net: "udp", UDPSize: uint16(bufsize)}
	}
	return nil
}

type DNSServer struct {
	Zone        Zone
	Iface       *net.Interface
	Upstream    *dns.ClientConfig
	udpSrv      *dns.Server
	tcpSrv      *dns.Server
	mdnsCli     *MDNSClient
	mdnsSrv     *MDNSServer
	cache       *Cache
	timeout     int
	udpBuf      int
	listenersWg *sync.WaitGroup

	Domain     string // the local domain
	ListenAddr string // the address the server is listening at
}

// Creates a new DNS server
func NewDNSServer(config DNSServerConfig, zone Zone, iface *net.Interface) (s *DNSServer, err error) {
	s = &DNSServer{
		Zone:        zone,
		Iface:       iface,
		listenersWg: new(sync.WaitGroup),

		Domain:     DefaultLocalDomain,
		ListenAddr: fmt.Sprintf(":%d", DefaultServerPort),
	}

	// fill empty parameters with defaults...
	if config.Port != 0 {
		s.ListenAddr = fmt.Sprintf(":%d", config.Port)
	}
	if len(config.LocalDomain) > 0 {
		s.Domain = config.LocalDomain
	}
	if config.UpstreamCfg != nil {
		s.Upstream = config.UpstreamCfg
	} else {
		cfgFile := DefaultCLICfgFile
		if len(config.UpstreamCfgFile) > 0 {
			cfgFile = config.UpstreamCfgFile
		}
		if s.Upstream, err = dns.ClientConfigFromFile(cfgFile); err != nil {
			return nil, err
		}
	}
	if config.Timeout > 0 {
		s.timeout = config.Timeout
	}
	if config.UDPBufLen > 0 {
		s.udpBuf = config.UDPBufLen
	}
	s.mdnsCli, err = NewMDNSClient()
	if err != nil {
		return
	}
	s.mdnsSrv, err = NewMDNSServer(s.Zone)
	if err != nil {
		return
	}
	cacheLen := DefaultCacheLen
	if config.CacheLen > 0 {
		cacheLen = config.CacheLen
	}
	Debug.Printf("[dns] Initializing cache: %d entries", config.CacheLen)
	s.cache, err = NewCache(cacheLen, nil)
	if err != nil {
		return
	}

	// create two DNS request multiplexerers, depending on the protocol used by clients
	// (we use the same protocol for asking upstream servers)
	mux := func(proto dnsProtocol) *dns.ServeMux {
		m := dns.NewServeMux()
		m.HandleFunc(s.Domain, s.queryHandler(proto))
		m.HandleFunc(RDNSDomain, s.rdnsHandler(proto))
		m.HandleFunc(".", s.notUsHandler(proto))
		return m
	}

	s.udpSrv = &dns.Server{Addr: s.ListenAddr, Net: "udp", Handler: mux(protUDP)}
	s.tcpSrv = &dns.Server{Addr: s.ListenAddr, Net: "tcp", Handler: mux(protTCP)}

	return
}

// Start the DNS server
func (s *DNSServer) Start() error {
	Info.Printf("Using mDNS on %v", s.Iface)
	err := s.mdnsCli.Start(s.Iface)
	CheckFatal(err)
	err = s.mdnsSrv.Start(s.Iface)
	CheckFatal(err)

	s.listenersWg.Add(2)

	go func() {
		defer s.listenersWg.Done()

		Debug.Printf("Listening for DNS on %s (UDP)", s.ListenAddr)
		err = s.udpSrv.ListenAndServe()
		CheckFatal(err)
		Debug.Printf("DNS UDP server exiting...")
	}()

	go func() {
		defer s.listenersWg.Done()

		Debug.Printf("Listening for DNS on %s (TCP)", s.ListenAddr)
		err = s.tcpSrv.ListenAndServe()
		CheckFatal(err)
		Debug.Printf("DNS TCP server exiting...")
	}()

	// Waiting for all goroutines to finish (otherwise they die as main routine dies)
	s.listenersWg.Wait()

	Info.Printf("WeaveDNS server exiting...")
	return nil
}

// Return status string
func (s *DNSServer) Status() string {
	var buf bytes.Buffer
	fmt.Fprintln(&buf, "Local domain", s.Domain)
	fmt.Fprintln(&buf, "Listen address", s.ListenAddr)
	fmt.Fprintln(&buf, "mDNS interface", s.Iface)
	fmt.Fprintln(&buf, "Fallback DNS config", s.Upstream)
	fmt.Fprintf(&buf, "Zone database:\n%s", s.Zone)
	return buf.String()
}

// Perform a graceful shutdown
func (s *DNSServer) Stop() error {
	// Stop the listeners/handlers
	if err := s.tcpSrv.Shutdown(); err != nil {
		return err
	}
	if err := s.udpSrv.Shutdown(); err != nil {
		return err
	}
	s.listenersWg.Wait()

	// shutdown the mDNS server
	s.mdnsSrv.Stop()

	return nil
}

func (s *DNSServer) queryHandler(proto dnsProtocol) dns.HandlerFunc {
	zoneLookup := func(lookup ZoneLookup, q *dns.Question, r *dns.Msg) (*dns.Msg, []ZoneRecord, error) {
		if ips, err := lookup.LookupName(q.Name); err != nil {
			return nil, nil, err
		} else {
			return makeAddressReply(r, q, ips), ips, nil
		}
	}

	fallback := func(w dns.ResponseWriter, r *dns.Msg) {
		w.WriteMsg(makeDNSFailResponse(r))
	}

	return s.commonQueryHandler(proto, "Query", dns.TypeA, zoneLookup, fallback)
}

func (s *DNSServer) rdnsHandler(proto dnsProtocol) dns.HandlerFunc {
	zoneLookup := func(lookup ZoneLookup, q *dns.Question, r *dns.Msg) (*dns.Msg, []ZoneRecord, error) {
		if names, err := lookup.LookupInaddr(q.Name); err != nil {
			return nil, nil, err
		} else {
			return makePTRReply(r, q, names), names, nil
		}
	}

	notUsHandler := s.notUsHandler(proto)
	fallback := func(w dns.ResponseWriter, r *dns.Msg) {
		Info.Printf("[dns msgid %d] -> sending to fallback server", r.MsgHdr.Id)
		notUsHandler(w, r)
	}

	return s.commonQueryHandler(proto, "Reverse query", dns.TypePTR, zoneLookup, fallback)
}

func (s *DNSServer) commonQueryHandler(proto dnsProtocol, kind string, qtype uint16,
	zoneLookup func(ZoneLookup, *dns.Question, *dns.Msg) (*dns.Msg, []ZoneRecord, error), fallback dns.HandlerFunc) dns.HandlerFunc {

	return func(w dns.ResponseWriter, r *dns.Msg) {
		q := r.Question[0]
		Debug.Printf("[dns msgid %d] %s: %+v", r.MsgHdr.Id, kind, q)
		maxLen := getMaxReplyLen(r, proto)
		lookups := []ZoneLookup{s.Zone, s.mdnsCli}

		reply, err := s.cache.Get(r, maxLen)
		if err != nil {
			if err == errNoLocalReplies {
				Debug.Printf("[dns msgid %d] Cached 'no local replies' - skipping local lookup", r.MsgHdr.Id)
				lookups = []ZoneLookup{s.Zone}
			} else {
				Debug.Printf("[dns msgid %d] Error from cache: %s", r.MsgHdr.Id, err)
				w.WriteMsg(makeDNSFailResponse(r))
				return
			}
		}
		if reply != nil {
			Debug.Printf("[dns msgid %d] Returning reply from cache: %s/%d answers",
				r.MsgHdr.Id, dns.RcodeToString[reply.MsgHdr.Rcode], len(reply.Answer))
			w.WriteMsg(reply)
			return
		}

		// catch unsupported queries
		if q.Qtype != qtype {
			Debug.Printf("[dns msgid %d] Unsupported query type %s", r.MsgHdr.Id, dns.TypeToString[q.Qtype])
			m := makeDNSNotImplResponse(r)
			s.cache.Put(r, m, negLocalTTL, 0)
			w.WriteMsg(m)
			return
		}

		for _, lookup := range lookups {
			if m, answers, err := zoneLookup(lookup, &q, r); err == nil {
				m.Authoritative = true
				Debug.Printf("[dns msgid %d] Caching response for type %s query for '%s': %s [code:%s]",
					m.MsgHdr.Id, dns.TypeToString[q.Qtype], q.Name, answers, dns.RcodeToString[m.Rcode])
				s.cache.Put(r, m, nullTTL, 0)
				w.WriteMsg(m)
				return
			}
		}

		Info.Printf("[dns msgid %d] No results for type %s query for '%s' [caching no-local]",
			r.MsgHdr.Id, dns.TypeToString[q.Qtype], q.Name)
		s.cache.Put(r, nil, negLocalTTL, CacheNoLocalReplies)

		fallback(w, r)
	}
}

// When we receive a request for a name outside of our '.weave.local.'
// domain, ask the configured DNS server as a fallback.
func (s *DNSServer) notUsHandler(proto dnsProtocol) dns.HandlerFunc {
	dnsClient := proto.GetNewClient(DefaultUDPBuflen)

	return func(w dns.ResponseWriter, r *dns.Msg) {
		maxLen := getMaxReplyLen(r, proto)
		q := r.Question[0]

		// create a request where we announce our max payload size
		rcopy := r
		rcopy.SetEdns0(uint16(maxLen), false)

		Debug.Printf("[dns msgid %d] Fallback query: %+v [%s, max:%d bytes]", rcopy.MsgHdr.Id, q, proto, maxLen)
		for _, server := range s.Upstream.Servers {
			reply, _, err := dnsClient.Exchange(rcopy, fmt.Sprintf("%s:%s", server, s.Upstream.Port))
			if err != nil {
				Debug.Printf("[dns msgid %d] Network error trying %s (%s)",
					r.MsgHdr.Id, server, err)
				continue
			}
			if reply != nil && reply.Rcode != dns.RcodeSuccess {
				Debug.Printf("[dns msgid %d] Failure reported by %s for query %s",
					r.MsgHdr.Id, server, q.Name)
				continue
			}
			Debug.Printf("[dns msgid %d] Given answer by %s for query %s",
				r.MsgHdr.Id, server, q.Name)
			w.WriteMsg(reply)
			return
		}
		Warning.Printf("[dns msgid %d] Failed lookup for external name %s", r.MsgHdr.Id, q.Name)
		w.WriteMsg(makeDNSFailResponse(r))
	}
}
