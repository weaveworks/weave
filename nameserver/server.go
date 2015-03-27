package nameserver

import (
	"bytes"
	"fmt"
	"github.com/miekg/dns"
	. "github.com/zettio/weave/common"
	"net"
	"sync"
	"time"
)

const (
	DefaultLocalDomain   = "weave.local."     // The default name used for the local domain
	DefaultServerPort    = 53                 // The default server port
	DefaultCLICfgFile    = "/etc/resolv.conf" // default "resolv.conf" file to try to load
	DefaultUDPBuflen     = 4096               // bigger than the default 512
	DefaultCacheLen      = 8192               // default cache capacity
	DefaultResolvWorkers = 8                  // default number of resolution workers
	DefaultTimeout       = 5                  // default timeout for DNS resolutions
	DefaultIfaceName     = "default interface"
	DefaultResolvTries   = 3 // max # of times a worker tries to resolve a query
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
	udpSrv      *dns.Server
	tcpSrv      *dns.Server
	mdnsCli     *MDNSClient
	mdnsSrv     *MDNSServer
	cache       *Cache
	zone        Zone
	iface       *net.Interface
	upstream    *dns.ClientConfig
	timeout     int
	udpBuf      int
	listenersWg *sync.WaitGroup

	Domain     string // the local domain
	IfaceName  string // the interface where mDNS is working on
	ListenAddr string // the address the server is listening at
}

// Creates a new DNS server
func NewDNSServer(config DNSServerConfig, zone Zone, iface *net.Interface) (s *DNSServer, err error) {
	s = &DNSServer{
		zone:        zone,
		iface:       iface,
		listenersWg: new(sync.WaitGroup),

		Domain:     DefaultLocalDomain,
		IfaceName:  DefaultIfaceName,
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
		s.upstream = config.UpstreamCfg
	} else {
		cfgFile := DefaultCLICfgFile
		if len(config.UpstreamCfgFile) > 0 {
			cfgFile = config.UpstreamCfgFile
		}
		if s.upstream, err = dns.ClientConfigFromFile(cfgFile); err != nil {
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
	s.mdnsSrv, err = NewMDNSServer(s.zone)
	if err != nil {
		return
	}
	if iface != nil {
		s.IfaceName = iface.Name
	}
	cacheLen := DefaultCacheLen
	if config.CacheLen > 0 {
		cacheLen = config.CacheLen
	}
	Debug.Printf("[dns] Initializing cache: %d entries", config.CacheLen)
	s.cache, err = NewCache(cacheLen)
	if err != nil {
		return
	}

	// create two DNS request multiplexerers, depending on the protocol used by clients
	// (we use the same protocol for asking upstream servers)
	mux := func(proto dnsProtocol) *dns.ServeMux {
		m := dns.NewServeMux()
		m.HandleFunc(s.Domain, s.queryHandler([]Lookup{s.zone, s.mdnsCli}, proto))
		m.HandleFunc(RDNSDomain, s.rdnsHandler([]Lookup{s.zone, s.mdnsCli}, proto))
		m.HandleFunc(".", s.notUsHandler(proto))
		return m
	}

	s.udpSrv = &dns.Server{Addr: s.ListenAddr, Net: "udp", Handler: mux(protUDP)}
	s.tcpSrv = &dns.Server{Addr: s.ListenAddr, Net: "tcp", Handler: mux(protTCP)}

	return
}

// Start the DNS server
func (s *DNSServer) Start() error {
	Info.Printf("Using mDNS on %s", s.IfaceName)
	err := s.mdnsCli.Start(s.iface)
	CheckFatal(err)
	err = s.mdnsSrv.Start(s.iface, s.Domain)
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
	fmt.Fprintln(&buf, "mDNS interface", s.iface)
	fmt.Fprintln(&buf, "Fallback DNS config", s.upstream)
	fmt.Fprintf(&buf, "Zone database:\n%s", s.zone)
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

func (s *DNSServer) queryHandler(lookups []Lookup, proto dnsProtocol) dns.HandlerFunc {
	return func(w dns.ResponseWriter, r *dns.Msg) {
		now := time.Now()
		q := r.Question[0]
		maxLen := getMaxReplyLen(r, proto)
		Debug.Printf("Query: %+v", q)

		if q.Qtype != dns.TypeA {
			Debug.Printf("[dns msgid %d] Unsuported query type %s", r.MsgHdr.Id, dns.TypeToString[q.Qtype])
			m := makeDNSNotImplResponse(r)
			s.cache.Put(r, m, 0, now)
			w.WriteMsg(m)
			return
		}

		reply, err := s.cache.Get(r, maxLen, time.Now())
		if err != nil {
			Debug.Printf("[dns msgid %d] Error from cache: %s", r.MsgHdr.Id, err)
			w.WriteMsg(makeDNSFailResponse(r))
			return
		}
		if reply != nil {
			Debug.Printf("[dns msgid %d] Returning reply from cache: %s/%d answers",
				r.MsgHdr.Id, dns.RcodeToString[reply.MsgHdr.Rcode], len(reply.Answer))
			w.WriteMsg(reply)
			return
		}
		for _, lookup := range lookups {
			if ip, err := lookup.LookupName(q.Name); err == nil {
				m := makeAddressReply(r, &q, []net.IP{ip})

				Debug.Printf("[dns msgid %d] Caching response for %s-query for \"%s\": %s [code:%s]",
					m.MsgHdr.Id, dns.TypeToString[q.Qtype], q.Name, ip, dns.RcodeToString[m.Rcode])
				s.cache.Put(r, m, 0, now)
				w.WriteMsg(m)
				return
			}
			now = time.Now()
		}

		Info.Printf("[dns msgid %d] No results for type %s query %s",
			r.MsgHdr.Id, dns.TypeToString[q.Qtype], q.Name)
		m := makeDNSFailResponse(r)
		s.cache.Put(r, m, 0, now)
		w.WriteMsg(m)
	}
}

func (s *DNSServer) rdnsHandler(lookups []Lookup, proto dnsProtocol) dns.HandlerFunc {
	fallback := s.notUsHandler(proto)
	return func(w dns.ResponseWriter, r *dns.Msg) {
		now := time.Now()
		q := r.Question[0]
		maxLen := getMaxReplyLen(r, proto)
		Debug.Printf("Reverse query: %+v", q)

		if q.Qtype != dns.TypePTR {
			Warning.Printf("[dns msgid %d] Unexpected reverse query type %s: %+v",
				r.MsgHdr.Id, dns.TypeToString[q.Qtype], q)
			m := makeDNSNotImplResponse(r)
			s.cache.Put(r, m, 0, now)
			w.WriteMsg(m)
			return
		}

		reply, err := s.cache.Get(r, maxLen, time.Now())
		if err != nil {
			if err == errNoLocalReplies {
				Debug.Printf("[dns msgid %d] Cached 'no local replies' - skipping local lookup", r.MsgHdr.Id)
				fallback(w, r)
			} else {
				Debug.Printf("[dns msgid %d] Error from cache: %s", r.MsgHdr.Id, err)
				w.WriteMsg(makeDNSFailResponse(r))
			}
			return
		}
		if reply != nil {
			Debug.Printf("[dns msgid %d] Returning reply from cache: %s/%d answers",
				r.MsgHdr.Id, dns.RcodeToString[reply.MsgHdr.Rcode], len(reply.Answer))
			w.WriteMsg(reply)
			return
		}
		for _, lookup := range lookups {
			if name, err := lookup.LookupInaddr(q.Name); err == nil {
				m := makePTRReply(r, &q, []string{name})
				Debug.Printf("[dns msgid %d] Caching response for %s-query for \"%s\": %s [code:%s]",
					m.MsgHdr.Id, dns.TypeToString[q.Qtype], q.Name, name, dns.RcodeToString[m.Rcode])
				s.cache.Put(r, m, 0, now)
				w.WriteMsg(m)
				return
			}
			now = time.Now()
		}

		s.cache.Put(r, nil, CacheNoLocalReplies, now)
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
		for _, server := range s.upstream.Servers {
			reply, _, err := dnsClient.Exchange(rcopy, fmt.Sprintf("%s:%s", server, s.upstream.Port))
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
		Warning.Printf("[dns msgid %d] Failed lookup for external name %s",
			r.MsgHdr.Id, q.Name)
		w.WriteMsg(makeDNSFailResponse(r))
	}
}
