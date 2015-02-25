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
	DEFAULT_CACHE_LEN      = 1024               // default cache capacity
	DEFAULT_RESOLV_WORKERS = 8                  // default number of resolution workers
	DEFAULT_TIMEOUT        = 5                  // default timeout for DNS resolutions
	DEFAULT_IFACE_NAME   = "default interface"
    DEFAULT_RESOLV_TRIES   = 3                  // max # of times a worker tries to resolve a query
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
	// (Optional) number of resolution workers for local queries
	NumLocalWorkers int
	// (Optional) number of resolution workers for recursive queries
	NumRecursiveWorkers int
	// (Optional) timeout for DNS queries
	Timeout int
	// (Optional) UDP buffer length
	UdpBufLen int
}

// a request for a resolution worker
type dnsWorkItem struct {
	protocol string   // the protocol where we received this query
	r        *dns.Msg // the query
}

type DNSServer struct {
	udpSrv   *dns.Server
	tcpSrv   *dns.Server
	mdnsCli  *MDNSClient
	mdnsSrv  *MDNSServer
	cache    *Cache
	zone     Zone
	iface    *net.Interface
	upstream *dns.ClientConfig
	timeout  int
	udpBuf   int

	numRecWorkers    int
	numLocalWorkers  int
	localQueriesChan chan dnsWorkItem // channel for sending queries to workers
	recQueriesChan   chan dnsWorkItem // ... and the equivalent for recursive resolutions
	workersWg        *sync.WaitGroup
	listenersWg      *sync.WaitGroup

	Domain     string // the local domain
	IfaceName  string // the interface where mDNS is working on
	ListenAddr string // the address the server is listening at
}

// Creates a new DNS server
func NewDNSServer(config DNSServerConfig, zone Zone, iface *net.Interface) (s *DNSServer, err error) {
	s = &DNSServer{
		zone:    zone,
		iface:   iface,
		timeout: DEFAULT_TIMEOUT,
		udpBuf:  DEFAULT_UDP_BUFLEN,
		
		numLocalWorkers:  DEFAULT_RESOLV_WORKERS,
		numRecWorkers:    DEFAULT_RESOLV_WORKERS,
		localQueriesChan: make(chan dnsWorkItem),
		recQueriesChan:   make(chan dnsWorkItem),
		workersWg:        new(sync.WaitGroup),
		listenersWg:      new(sync.WaitGroup),

		Domain:     DEFAULT_LOCAL_DOMAIN,
		IfaceName:  DEFAULT_IFACE_NAME,
		ListenAddr: fmt.Sprintf(":%d", DEFAULT_SERVER_PORT),
	}

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
	if config.NumLocalWorkers > 0 {
		s.numLocalWorkers = config.NumLocalWorkers
	}
	if config.NumRecursiveWorkers > 0 {
		s.numRecWorkers = config.NumRecursiveWorkers
	}
	s.mdnsServer, err := NewMDNSServer(config.Zone)
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
	if cacheLen < s.numLocalWorkers + s.numRecWorkers {
		// make sure the cache is big enough for all the workers
		cacheLen = s.numLocalWorkers + s.numRecWorkers
	}
	Debug.Printf("[dns] Initializing cache: %d entries", config.CacheLen)
	s.cache, err := NewCache(cacheLen)
	if err != nil {
		return
	}

	// create two DNS request multiplexerers, depending on the protocol used by clients
	// (we use the same protocol for asking upstream servers)
	udpMux := dns.NewServeMux()
	udpMux.HandleFunc(s.config.LocalDomain, s.makeHandler("udp", s.localQueriesChan))
	udpMux.HandleFunc(RDNS_DOMAIN, s.makeHandler("udp", s.localQueriesChan))
	udpMux.HandleFunc(".", s.makeHandler("udp", s.recQueriesChan))

	tcpMux := dns.NewServeMux()
	tcpMux.HandleFunc(s.config.LocalDomain, s.makeHandler("tcp", s.localQueriesChan))
	tcpMux.HandleFunc(RDNS_DOMAIN, s.makeHandler("tcp", s.localQueriesChan))
	tcpMux.HandleFunc(".", s.makeHandler("tcp", s.recQueriesChan))

	s.udpSrv = &dns.Server{Addr: s.ListenAddr, Net: "udp", Handler: udpMux}
	s.tcpSrv = &dns.Server{Addr: s.ListenAddr, Net: "tcp", Handler: tcpMux}

	return
}

// return a handler for a given protocol and channel
func (s *DNSServer) makeHandler(protocol string, queriesChan chan<- dnsWorkItem) dns.HandlerFunc {
	tout := time.Duration(s.timeout) * time.Second

	return func(w dns.ResponseWriter, r *dns.Msg) {
		now := time.Now()
		resolveUntil := now.Add(tout)
		remTries := DEFAULT_RESOLV_TRIES

		// this loop can exit 1) due to a reply!=nil 2) with a timeout on Wait()
		for {
			q := r.Question[0]
			Debug.Printf("[dns] Query: %+v", q)
			reply, err := s.cache.Get(r, now)
			if err != nil {
				Debug.Printf("[dns msgid %d] Error from cache: %s", r.MsgHdr.Id, err)
				w.WriteMsg(makeDNSFailResponse(r))
				return
			}
			if reply != nil {
				if reply.Truncated && protocol == "tcp" {
					Debug.Printf("[dns msgid %d] Truncated response: invalidating cache",
						r.MsgHdr.Id)
					s.cache.Invalidate(r, now)
				} else {
					Debug.Printf("[dns msgid %d] Returning reply from cache: %d answers",
						r.MsgHdr.Id, len(reply.Answer))
					w.WriteMsg(reply)
					return
				}
			}

			// we got no reply and no error from the cache: send the query to a worker and wait
			queriesChan <- dnsWorkItem{protocol: protocol, r: r}
			remTime := resolveUntil.Sub(now)
			Debug.Printf("[dns] Waiting up to %.2f secs for %s-query for \"%s\"",
				remTime.Seconds(), dns.TypeToString[q.Qtype], q.Name)
			reply, err = s.cache.Wait(r, remTime, now)
			if err != nil {
				if err == errTimeout {
					Debug.Printf("[dns msgid %d] Timeout while waiting for response", r.MsgHdr.Id)
				} else {
					Debug.Printf("[dns msgid %d] Error from cache: %s", r.MsgHdr.Id, err)
				}
				w.WriteMsg(makeDNSFailResponse(r))
				return
			}
			if reply != nil {
				Info.Printf("[dns msgid %d] Returning reply from cache: %d answers",
					r.MsgHdr.Id, len(reply.Answer))
				w.WriteMsg(reply)
				return
			}

			remTries -= 1
			if remTries == 0 {
				Info.Printf("[dns msgid %d] Too many tries for %s-query for \"%s\"",
					r.MsgHdr.Id, dns.TypeToString[q.Qtype], q.Name)
				w.WriteMsg(makeDNSFailResponse(r))
				return
			}

			Info.Printf("[dns msgid %d] No results for %s-query for \"%s\": retrying...",
				r.MsgHdr.Id, dns.TypeToString[q.Qtype], q.Name)
			now = time.Now()
		}
	}
}

// Start the DNS server
func (s *DNSServer) Start() error {
	Info.Printf("[dns] Server starting...")

	Debug.Printf("[dns] Starting mDNS server on %s", s.IfaceName)
	err := s.mdnsSrv.Start(s.iface, s.Domain)
	if err != nil {
		Error.Printf("[dns] Could not start mDNS server: %s", err)
		return err
	}

	s.listenersWg.Add(2)

	go func() {
		defer s.listenersWg.Done()
		Debug.Printf("[dns] Listening on %s (UDP)", s.ListenAddr)
		err = s.udpSrv.ListenAndServe()
		CheckFatal(err)
		Debug.Printf("[dns] UDP server exiting...")
	}()

	go func() {
		defer s.listenersWg.Done()
		Debug.Printf("[dns] Listening on %s (TCP)", s.ListenAddr)
		err = s.tcpSrv.ListenAndServe()
		CheckFatal(err)
		Debug.Printf("[dns] Exiting TCP server...")
	}()

	// Start the resolution workers
	for i := 0; i < s.config.NumLocalWorkers; i++ {
		s.workersWg.Add(1)
		go s.localLookupWorker(i)
	}
	for i := 0; i < s.config.NumRecursiveWorkers; i++ {
		s.workersWg.Add(1)
		go s.recLookupWorker(i)
	}

	// Wait for all goroutines to finish
	s.workersWg.Wait()
	s.listenersWg.Wait()

	Info.Printf("[dns] Server exiting...")
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

	// stop the workers by closing the items channels and wait for them...
	close(s.localQueriesChan)
	close(s.recQueriesChan)
	s.workersWg.Wait()

	// shutdown the mDNS server
	s.mdnsSrv.Stop()

	return nil
}

// Worker for local resolutions
func (s *DNSServer) localLookupWorker(numWorker int) {
	defer s.workersWg.Done()

	// each local worker has its own mDNS client for resolving queries
	mdnsCli, err := NewMDNSClient()
	if err != nil {
		Error.Printf("[dns] Could not initialize mDNS client in local worker: %s", err)
		return
	}
	err = mdnsCli.Start(s.config.Iface)
	if err != nil {
		Error.Printf("[dns] Could not start mDNS client in local worker: %s", err)
		return
	}

	Debug.Printf("[dns] Starting local queries worker #%d", numWorker)
	for query := range s.localQueriesChan {
		r := query.r
		q := r.Question[0]
		resolved := false
		now := time.Now()

		Debug.Printf("[dns msgid %d] Resolving local %s-query for \"%+v\" [proto:%s]",
			r.MsgHdr.Id, dns.TypeToString[q.Qtype], q.Name, query.protocol)

		switch q.Qtype {
		case dns.TypeA:
			for _, lookup := range []Lookup{s.config.Zone, mdnsCli} {
				if ip, err := lookup.LookupName(q.Name); err == nil {
					m := makeAddressReply(r, &q, []net.IP{ip})

					Debug.Printf("[dns msgid %d] Caching response for %s-query for \"%s\": %s [code:%s]",
						m.MsgHdr.Id, dns.TypeToString[q.Qtype], q.Name, ip, dns.RcodeToString[m.Rcode])
					s.cache.Put(r, m, CacheLocalReply, now)
					resolved = true
					break
				}
			}
			if !resolved {
				Info.Printf("[dns msgid %d] No local results for %s-query for \"%s\" [proto:%s] [caching error]",
					r.MsgHdr.Id, dns.TypeToString[q.Qtype], q.Name, query.protocol)
				s.cache.Put(r, makeDNSFailResponse(r), CacheLocalReply, now)
			}

		case dns.TypePTR:
			for _, lookup := range []Lookup{s.config.Zone, mdnsCli} {
				if name, err := lookup.LookupInaddr(q.Name); err == nil {
					m := makePTRReply(r, &q, []string{name})

					Debug.Printf("[dns msgid %d] Caching response for %s-query for \"%s\": %s [code:%s]",
						m.MsgHdr.Id, dns.TypeToString[q.Qtype], q.Name, name, dns.RcodeToString[m.Rcode])
					s.cache.Put(r, m, CacheLocalReply, now)
					resolved = true
					break
				}
			}
			if !resolved {
				Debug.Printf("[dns msgid %d] Falling back to recursive resolution for %s-query for \"%s\"",
					r.MsgHdr.Id, dns.TypeToString[q.Qtype], q.Name)
				s.recQueriesChan <- query
			}

		default:
			Info.Printf("[dns msgid %d] Unhandled %s-query for \"%s\" [proto:%s] [caching error]",
				r.MsgHdr.Id, dns.TypeToString[q.Qtype], q.Name, query.protocol)
			s.cache.Put(r, makeDNSFailResponse(r), 0, now)
		}
	}

	Debug.Printf("[dns] Exiting local queries worker #%d", numWorker)
}

// Worker for recursive resolutions
func (s *DNSServer) recLookupWorker(numWorker int) {
	defer s.workersWg.Done()

	// each worker can use one of these two clients for forwarding queries, depending
	// on the protocol used by the client for querying us...
	udpClient := &dns.Client{Net: "udp", UDPSize: s.udpBuf}
	tcpClient := &dns.Client{Net: "tcp"}

	// When we receive a request for a name outside of our '.weave.local.'
	// domain, ask the configured DNS server as a fallback.
	Debug.Printf("[dns] Starting recursive queries worker #%d", numWorker)
	for query := range s.recQueriesChan {
		r := query.r
		proto := query.protocol
		q := r.Question[0]
		resolved := false
		now := time.Now()

		var dnsClient *dns.Client
		switch proto {
		case "tcp":
			dnsClient = tcpClient
		case "udp":
			dnsClient = udpClient
		}

		Debug.Printf("[dns msgid %d] Fallback query: %+v [proto:%s]", r.MsgHdr.Id, q, proto)
		for _, server := range s.config.UpstreamCfg.Servers {
			reply, _, err := dnsClient.Exchange(r, fmt.Sprintf("%s:%s", server, s.config.UpstreamCfg.Port))
			if err != nil {
				Debug.Printf("[dns msgid %d] Network error trying \"%s\": %s",
					r.MsgHdr.Id, server, err)
				continue
			}
			if reply != nil && reply.Rcode != dns.RcodeSuccess {
				Debug.Printf("[dns msgid %d] Failure reported by \"%s\" for query \"%s\": %s",
					r.MsgHdr.Id, server, q.Name, dns.RcodeToString[reply.Rcode])
				continue
			}

			Debug.Printf("[dns msgid %d] Given answer by %s for %s-query \"%s\": %d answers [caching response]",
				r.MsgHdr.Id, server, dns.TypeToString[q.Qtype], q.Name, len(reply.Answer))
			s.cache.Put(r, reply, 0, now)
			resolved = true
			break
		}

		if !resolved {
			Warning.Printf("[dns msgid %d] Failed recursive lookup for external name \"%s\" [caching error]",
				r.MsgHdr.Id, q.Name)
			s.cache.Put(r, makeDNSFailResponse(r), 0, now)
		}
	}

	Debug.Printf("[dns] Exiting recursive queries worker #%d", numWorker)
}
