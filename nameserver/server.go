package nameserver

import (
	"fmt"
	"github.com/miekg/dns"
	. "github.com/zettio/weave/common"
	"net"
	"sync"
	"time"
)

const (
	DEFAULT_LOCAL_DOMAIN   = "weave.local."     // The default name used for the local domain
	DEFAULT_SERVER_PORT    = 53                 // The default server port
	DEFAULT_CLI_CFG_FILE   = "/etc/resolv.conf" // default "resolv.conf" file to try to load
	DEFAULT_UDP_BUFLEN     = 4096               // bigger than the default 512
	DEFAULT_CACHE_LEN      = 8192               // default cache capacity
	DEFAULT_RESOLV_WORKERS = 8                  // default number of resolution workers
	DEFAULT_TIMEOUT        = 5                  // default timeout for DNS resolutions
	DEFAULT_IFACE_NAME     = "default interface"
	DEFAULT_RESOLV_TRIES   = 3 // max # of times a worker tries to resolve a query
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
	UdpBufLen int
}

type dnsProtocol uint8

const (
	protUdp dnsProtocol = iota // UDP protocol
	protTcp dnsProtocol = iota // TCP protocol
)

func (proto dnsProtocol) String() string {
	switch proto {
	case protUdp:
		return "UDP"
	case protTcp:
		return "TCP"
	}
	return "unknown"
}

// get a new dns.Client for a protocol
func (proto dnsProtocol) GetNewClient(bufsize int) *dns.Client {
	switch proto {
	case protTcp:
		return &dns.Client{Net: "tcp"}
	case protUdp:
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

		Domain:     DEFAULT_LOCAL_DOMAIN,
		IfaceName:  DEFAULT_IFACE_NAME,
		ListenAddr: fmt.Sprintf(":%d", DEFAULT_SERVER_PORT),
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
		cfgFile := DEFAULT_CLI_CFG_FILE
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
	if config.UdpBufLen > 0 {
		s.udpBuf = config.UdpBufLen
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
	cacheLen := DEFAULT_CACHE_LEN
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
		m.HandleFunc(RDNS_DOMAIN, s.rdnsHandler([]Lookup{s.zone, s.mdnsCli}, proto))
		m.HandleFunc(".", s.notUsHandler(proto))
		return m
	}

	s.udpSrv = &dns.Server{Addr: s.ListenAddr, Net: "udp", Handler: mux(protUdp)}
	s.tcpSrv = &dns.Server{Addr: s.ListenAddr, Net: "tcp", Handler: mux(protTcp)}

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
		q := r.Question[0]
		maxLen := getMaxReplyLen(r, proto)
		Debug.Printf("Query: %+v", q)
		if q.Qtype == dns.TypeA {
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
				now := time.Now()
				if ip, err := lookup.LookupName(q.Name); err == nil {
					m := makeAddressReply(r, &q, []net.IP{ip})

					Debug.Printf("[dns msgid %d] Caching response for %s-query for \"%s\": %s [code:%s]",
						m.MsgHdr.Id, dns.TypeToString[q.Qtype], q.Name, ip, dns.RcodeToString[m.Rcode])
					s.cache.Put(r, m, CacheLocalReply, now)
					w.WriteMsg(m)
					return
				}
			}
		}
		Info.Printf("[dns msgid %d] No results for type %s query %s",
			r.MsgHdr.Id, dns.TypeToString[q.Qtype], q.Name)
		w.WriteMsg(makeDNSFailResponse(r))
	}
}

func (s *DNSServer) rdnsHandler(lookups []Lookup, proto dnsProtocol) dns.HandlerFunc {
	fallback := s.notUsHandler(proto)
	return func(w dns.ResponseWriter, r *dns.Msg) {
		q := r.Question[0]
		maxLen := getMaxReplyLen(r, proto)
		Debug.Printf("Reverse query: %+v", q)
		if q.Qtype == dns.TypePTR {
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
				now := time.Now()
				if name, err := lookup.LookupInaddr(q.Name); err == nil {
					m := makePTRReply(r, &q, []string{name})
					Debug.Printf("[dns msgid %d] Caching response for %s-query for \"%s\": %s [code:%s]",
						m.MsgHdr.Id, dns.TypeToString[q.Qtype], q.Name, name, dns.RcodeToString[m.Rcode])
					s.cache.Put(r, m, CacheLocalReply, now)

					w.WriteMsg(m)
					return
				}
			}
			fallback(w, r)
			return
		}
		Warning.Printf("[dns msgid %d] Unexpected reverse query type %s: %+v",
			r.MsgHdr.Id, dns.TypeToString[q.Qtype], q)
	}
}

// When we receive a request for a name outside of our '.weave.local.'
// domain, ask the configured DNS server as a fallback.
func (s *DNSServer) notUsHandler(proto dnsProtocol) dns.HandlerFunc {
	udpClient := protUdp.GetNewClient(DEFAULT_UDP_BUFLEN)
	tcpClient := protTcp.GetNewClient(0)
	dnsClients := []*dns.Client{udpClient, tcpClient}

	return func(w dns.ResponseWriter, r *dns.Msg) {
		q := r.Question[0]
		Debug.Printf("[dns msgid %d] Fallback query: %+v", r.MsgHdr.Id, q)
		maxLen := getMaxReplyLen(r, proto)
		reply, err := s.cache.Get(r, maxLen, time.Now())
		if err != nil {
			Debug.Printf("[dns msgid %d] Error from cache: %s", r.MsgHdr.Id, err)
			w.WriteMsg(makeDNSFailResponse(r))
			return
		}
		if reply != nil {
			Debug.Printf("[dns msgid %d] Returning reply from cache: %s/%d answers [%d bytes]",
				r.MsgHdr.Id, dns.RcodeToString[reply.MsgHdr.Rcode], len(reply.Answer), reply.Len())
			w.WriteMsg(reply)
			return
		}

		// create a request where we announce our max payload size
		m := r
		m.SetEdns0(uint16(s.udpBuf), false)

	ServersLoop:
		for _, server := range s.upstream.Servers {
			for _, dnsClient := range dnsClients {
				reply, _, err := dnsClient.Exchange(m, fmt.Sprintf("%s:%s", server, s.upstream.Port))
				if err != nil {
					Debug.Printf("[dns msgid %d] Network error trying \"%s\": %s",
						r.MsgHdr.Id, server, err)
					continue ServersLoop
				}
				if reply != nil && reply.Rcode != dns.RcodeSuccess {
					Debug.Printf("[dns msgid %d] Failure reported by \"%s\" for query \"%s\": %s",
						r.MsgHdr.Id, server, q.Name, dns.RcodeToString[reply.Rcode])
					continue ServersLoop
				}
				if reply.Truncated {
					Debug.Printf("[dns msgid %d] Truncated response received from %s: retrying with TCP",
						r.MsgHdr.Id, server)
				} else {
					Debug.Printf("[dns msgid %d] Given answer by %s for %s-query \"%s\": %d answers [caching response]",
						r.MsgHdr.Id, server, dns.TypeToString[q.Qtype], q.Name, len(reply.Answer))
					replyLen := s.cache.Put(r, reply, 0, time.Now())

					// check if we can send the full response (otherwise, truncate)
					if replyLen > maxLen {
						Debug.Printf("[dns msgid %d] Reply too long for this client (%d > %d): truncating",
							r.MsgHdr.Id, replyLen, maxLen)
						reply = makeTruncatedReply(r)
					}
					w.WriteMsg(reply)
					return
				}
			}
		}

		Warning.Printf("[dns msgid %d] Failed lookup for external name %s",
			r.MsgHdr.Id, q.Name)

		reply = makeDNSFailResponse(r)
		s.cache.Put(r, reply, 0, time.Now())
		w.WriteMsg(reply)
	}
}
