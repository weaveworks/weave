package nameserver

import (
	"bytes"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/miekg/dns"
	. "github.com/weaveworks/weave/common"
)

const (
	DefaultServerPort = 53                 // The default server port
	DefaultHTTPPort   = 6785               // The default http port
	DefaultCLICfgFile = "/etc/resolv.conf" // default "resolv.conf" file to try to load
	DefaultLocalTTL   = 30                 // default TTL for responses for local domain queries
	DefaultUDPBuflen  = 4096               // bigger than the default 512
	DefaultCacheLen   = 8192               // default cache capacity
	DefaultTimeout    = 5000               // default timeout for DNS resolutions (millisecs)
	DefaultMaxAnswers = 1                  // default number of answers provided to users
)

type DNSServerConfig struct {
	// The zone
	Zone Zone
	// (Optional) client config file for resolving upstream servers
	UpstreamCfgFile string
	// (Optional) DNS client config for the fallback server(s)
	UpstreamCfg *dns.ClientConfig
	// (Optional) port number (for TCP and UDP)
	Port int
	// (Optional) cache size
	CacheLen int
	// (Optional) disable the cache
	CacheDisabled bool
	// (Optional) timeout for DNS queries
	Timeout int
	// (Optional) UDP buffer length
	UDPBufLen int
	// (Optional) force a specific cache
	Cache ZoneCache
	// (Optional) TTL for local domain responses
	LocalTTL int
	// (Optional) TTL for negative results in the local domain (defaults to LocalTTL)
	CacheNegLocalTTL int
	// (Optional) for a specific clock provider
	Clock clock.Clock
	// (Optional) Listening socket read timeout (in milliseconds)
	ListenReadTimeout int
	// Maximum number of answers provided to users
	MaxAnswers int
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
func (proto dnsProtocol) GetNewClient(bufsize int, timeout time.Duration) *dns.Client {
	Log.Debugf("[dns] Creating %s DNS client with %s timeout", proto, timeout)
	switch proto {
	case protTCP:
		return &dns.Client{Net: "tcp", ReadTimeout: timeout}
	case protUDP:
		return &dns.Client{Net: "udp", ReadTimeout: timeout, UDPSize: uint16(bufsize)}
	}
	return nil
}

// a DNS server
type DNSServer struct {
	Zone       Zone
	Upstream   *dns.ClientConfig
	Domain     string // the local domain
	ListenAddr string // the address the server is listening at

	udpSrv        *dns.Server
	tcpSrv        *dns.Server
	pc            net.PacketConn
	lst           net.Listener
	cache         ZoneCache
	cacheDisabled bool
	maxAnswers    int
	localTTL      int
	negLocalTTL   int
	timeout       time.Duration
	readTimeout   time.Duration
	udpBuf        int
	listenersWg   *sync.WaitGroup
	clock         clock.Clock
}

// Creates a new DNS server
func NewDNSServer(config DNSServerConfig) (s *DNSServer, err error) {
	s = &DNSServer{
		Zone:       config.Zone,
		Domain:     DefaultLocalDomain,
		ListenAddr: fmt.Sprintf(":%d", config.Port),

		listenersWg:   new(sync.WaitGroup),
		timeout:       DefaultTimeout * time.Millisecond,
		readTimeout:   DefaultTimeout * time.Millisecond,
		cacheDisabled: false,
		maxAnswers:    DefaultMaxAnswers,
		localTTL:      DefaultLocalTTL,
		clock:         config.Clock,
	}

	// check some basic parameters are valid
	if s.Zone == nil {
		return nil, fmt.Errorf("No valid Zone provided in server initialization")
	}
	if len(s.Domain) == 0 {
		return nil, fmt.Errorf("No valid Domain provided in server initialization")
	}
	if s.clock == nil {
		s.clock = clock.New()
	}

	// fill empty parameters with defaults...
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
		s.timeout = time.Duration(config.Timeout) * time.Millisecond
	}
	if config.ListenReadTimeout > 0 {
		s.readTimeout = time.Duration(config.ListenReadTimeout) * time.Millisecond
	}
	if config.UDPBufLen > 0 {
		s.udpBuf = config.UDPBufLen
	}
	if config.MaxAnswers > 0 {
		s.maxAnswers = config.MaxAnswers
	}
	if config.LocalTTL > 0 {
		s.localTTL = config.LocalTTL
	}
	if config.CacheNegLocalTTL > 0 {
		s.negLocalTTL = config.CacheNegLocalTTL
	} else {
		s.negLocalTTL = s.localTTL
	}
	if config.CacheDisabled {
		s.cacheDisabled = true
	}
	if !s.cacheDisabled {
		if config.Cache != nil {
			s.cache = config.Cache
		} else {
			cacheLen := DefaultCacheLen
			if config.CacheLen > 0 {
				cacheLen = config.CacheLen
			}
			if s.cache, err = NewCache(cacheLen, s.clock); err != nil {
				return
			}
		}
	}

	return
}

// Start the DNS server
func (s *DNSServer) Start() error {
	Info.Printf("[dns] Upstream server(s): %+v", s.Upstream)
	if s.cacheDisabled {
		Info.Printf("[dns] Cache: disabled")
	} else {
		Info.Printf("[dns] Cache: %d entries", s.cache.Capacity())
	}

	pc, err := net.ListenPacket("udp", s.ListenAddr)
	if err != nil {
		return err
	}
	s.pc = pc

	_, port, err := net.SplitHostPort(pc.LocalAddr().String())
	if err != nil {
		return err
	}
	s.ListenAddr = fmt.Sprintf(":%s", port)
	s.udpSrv = &dns.Server{PacketConn: s.pc, Handler: s.createMux(protUDP), ReadTimeout: s.readTimeout}

	// Bind the TCP socket at the same port, aborting otherwise
	l, err := net.Listen("tcp", s.ListenAddr)
	if err != nil {
		s.Stop()
		return err
	}
	s.lst = l
	s.tcpSrv = &dns.Server{Listener: l, Handler: s.createMux(protTCP), ReadTimeout: s.readTimeout}

	s.listenersWg.Add(2)
	return nil
}

func (s *DNSServer) ActivateAndServe() {
	go func() {
		defer s.listenersWg.Done()

		Info.Printf("[dns] Listening for DNS on %s (UDP)", s.ListenAddr)
		err := s.udpSrv.ActivateAndServe()
		CheckFatal(err)
		Log.Debugf("[dns] DNS UDP server exiting...")
	}()

	go func() {
		defer s.listenersWg.Done()

		Info.Printf("[dns] Listening for DNS on %s (TCP)", s.ListenAddr)
		err := s.tcpSrv.ActivateAndServe()
		CheckFatal(err)
		Log.Debugf("[dns] DNS TCP server exiting...")
	}()

	// Waiting for all goroutines to finish (otherwise they die as main routine dies)
	s.listenersWg.Wait()

	Info.Printf("[dns] Server exiting...")
}

// Return status string
func (s *DNSServer) Status() string {
	var buf bytes.Buffer
	fmt.Fprintln(&buf, "Listen address", s.ListenAddr)
	fmt.Fprintln(&buf, "Fallback DNS config", s.Upstream)
	return buf.String()
}

// Perform a graceful shutdown
func (s *DNSServer) Stop() error {
	// Stop the listeners/handlers
	if s.tcpSrv != nil {
		if err := s.tcpSrv.Shutdown(); err != nil {
			return err
		}
		s.lst.Close()
		s.tcpSrv = nil
	}
	if s.udpSrv != nil {
		if err := s.udpSrv.Shutdown(); err != nil {
			return err
		}
		s.pc.Close()
		s.udpSrv = nil
	}
	s.listenersWg.Wait()
	return nil
}

// Create a multiplexer for requests
// We must create two DNS request multiplexers, depending on the protocol used by
// clients (as we use the same protocol for asking upstream servers)
func (s *DNSServer) createMux(proto dnsProtocol) *dns.ServeMux {
	failFallback := func(w dns.ResponseWriter, r *dns.Msg) {
		w.WriteMsg(makeDNSFailResponse(r))
	}
	notUsHandler := s.notUsHandler(proto)
	notUsFallback := func(w dns.ResponseWriter, r *dns.Msg) {
		Info.Printf("[dns msgid %d] -> sending to fallback server", r.MsgHdr.Id)
		notUsHandler(w, r)
	}

	// create the multiplexer
	m := dns.NewServeMux()
	m.HandleFunc(s.Zone.Domain(), s.localHandler(proto, "Query", dns.TypeA,
		s.Zone.DomainLookupName, makeAddressReply, s.Zone.ObserveName, failFallback))
	m.HandleFunc(RDNSDomain, s.localHandler(proto, "Reverse query", dns.TypePTR,
		s.Zone.DomainLookupInaddr, makePTRReply, s.Zone.ObserveInaddr, notUsFallback))
	m.HandleFunc(".", s.notUsHandler(proto))
	return m
}

// Process a request for a name/address in the '.weave.local.' zone
func (s *DNSServer) localHandler(proto dnsProtocol, kind string, qtype uint16,
	lookup func(inaddr string) ([]ZoneRecord, error),
	msgBuilder DNSResponseBuilder, observer ZoneObserverFunc, fallback dns.HandlerFunc) dns.HandlerFunc {

	return func(w dns.ResponseWriter, r *dns.Msg) {
		q := r.Question[0]
		Log.Debugf("[dns msgid %d] %s: %+v", r.MsgHdr.Id, kind, q)
		maxLen := getMaxReplyLen(r, proto)

		// cache a response if the cache is enabled, and observe the name/IP
		maybeCache := func(m *dns.Msg, ttl int, flags uint8) {
			if !s.cacheDisabled {
				s.cache.Put(r, m, ttl, flags)
				// any change in the Zone database for this IP/name will lead to this
				// cache entry being removed...
				// TODO: this closure results in unnecessary `Remove`s and some wasted
				// mem... but we can live with that.
				observer(q.Name, func() { s.cache.Remove(&q) })
			}
		}

		if !s.cacheDisabled {
			reply, err := s.cache.Get(r, maxLen)
			if err != nil {
				if err == errNoLocalReplies {
					Log.Debugf("[dns msgid %d] Cached no-local-replies", r.MsgHdr.Id)
					fallback(w, r)
				} else {
					Log.Debugf("[dns msgid %d] Error from cache: %s", r.MsgHdr.Id, err)
					w.WriteMsg(makeDNSFailResponse(r))
				}
				return
			}
			if reply != nil {
				reply.Answer = pruneAnswers(shuffleAnswers(reply.Answer), s.maxAnswers)
				Log.Debugf("[dns msgid %d] Returning reply from cache: %s/%d answers",
					r.MsgHdr.Id, dns.RcodeToString[reply.MsgHdr.Rcode], len(reply.Answer))
				w.WriteMsg(reply)
				return
			}
		}

		// catch unsupported queries
		if q.Qtype != qtype {
			Log.Debugf("[dns msgid %d] Unsupported query type %s", r.MsgHdr.Id, dns.TypeToString[q.Qtype])
			m := makeDNSFailResponse(r)
			maybeCache(m, s.negLocalTTL, 0)
			w.WriteMsg(m)
			return
		}

		if answers, err := lookup(q.Name); err != nil {
			Info.Printf("[dns msgid %d] No results for type %s query for '%s' [caching no-local]",
				r.MsgHdr.Id, dns.TypeToString[q.Qtype], q.Name)
			maybeCache(nil, s.negLocalTTL, CacheNoLocalReplies)
			fallback(w, r)
		} else {
			m := msgBuilder(r, &q, answers, s.localTTL)
			m.Authoritative = true
			maybeCache(m, nullTTL, 0)
			m.Answer = pruneAnswers(shuffleAnswers(m.Answer), s.maxAnswers)
			Log.Debugf("[dns msgid %d] Sending response: %s/%d answers [code:%s]",
				m.MsgHdr.Id, dns.TypeToString[q.Qtype], len(m.Answer), dns.RcodeToString[m.Rcode])
			w.WriteMsg(m)
		}
	}
}

// When we receive a request for a name outside of our '.weave.local.'
// domain, ask the configured DNS server as a fallback.
func (s *DNSServer) notUsHandler(proto dnsProtocol) dns.HandlerFunc {
	dnsClient := proto.GetNewClient(DefaultUDPBuflen, s.timeout)

	return func(w dns.ResponseWriter, r *dns.Msg) {
		q := r.Question[0]

		// announce our max payload size as the max payload our client supports
		maxLen := getMaxReplyLen(r, proto)
		rcopy := r
		rcopy.SetEdns0(uint16(maxLen), false)

		Log.Debugf("[dns msgid %d] Fallback query: %+v [%s, max:%d bytes]", rcopy.MsgHdr.Id, q, proto, maxLen)
		for _, server := range s.Upstream.Servers {
			reply, _, err := dnsClient.Exchange(rcopy, fmt.Sprintf("%s:%s", server, s.Upstream.Port))
			if err != nil {
				Log.Debugf("[dns msgid %d] Network error trying %s (%s)",
					r.MsgHdr.Id, server, err)
				continue
			}
			if reply != nil && reply.Rcode != dns.RcodeSuccess {
				Log.Debugf("[dns msgid %d] Failure reported by %s for query %s",
					r.MsgHdr.Id, server, q.Name)
				continue
			}
			Log.Debugf("[dns msgid %d] Given answer by %s for query %s",
				r.MsgHdr.Id, server, q.Name)
			w.WriteMsg(reply)
			return
		}
		Log.Warningf("[dns msgid %d] Failed lookup for external name %s", r.MsgHdr.Id, q.Name)
		w.WriteMsg(makeDNSFailResponse(r))
	}
}

// Get the listen port
func (s *DNSServer) GetPort() (int, error) {
	_, portS, err := net.SplitHostPort(s.ListenAddr)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(portS)
}
