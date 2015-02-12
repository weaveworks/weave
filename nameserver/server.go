package nameserver

import (
	"fmt"
	"github.com/miekg/dns"
	. "github.com/zettio/weave/common"
	"net"
	"sync"
)

const (
	DEFAULT_LOCAL_DOMAIN = "weave.local."     // The default name used for the local domain
	DEFAULT_SERVER_PORT  = 53                 // The default server port
	DEFAULT_CLI_CFG_FILE = "/etc/resolv.conf" // default "resolv.conf" file to try to load
	UDPBufSize           = 4096               // bigger than the default 512
)

func makeDNSFailResponse(r *dns.Msg) *dns.Msg {
	m := new(dns.Msg)
	m.SetReply(r)
	m.RecursionAvailable = true
	m.Rcode = dns.RcodeNameError
	return m
}

type DNSServerConfig struct {
	// (Optional) client config file for resolving upstream servers
	UpstreamCfgFIle string
	// (Optional) DNS client config for the fallback server(s)
	UpstreamCfg *dns.ClientConfig
	// DNS zone served
	Zone Zone
	// Interface where serving
	Iface *net.Interface
	// (Optional) port number (for TCP and UDP)
	Port int
	// (Optional) local domain (ie, "weave.local.")
	LocalDomain string
}

type DNSServer struct {
	config  *DNSServerConfig
	udpSrv  *dns.Server
	tcpSrv  *dns.Server
	mdnsCli *MDNSClient
	mdnsSrv *MDNSServer

	IfaceName  string // the interface where mDNS is working on
	ListenAddr string // the address the server is listening at
}

// Creates a new DNS server
func NewDNSServer(config DNSServerConfig) (s *DNSServer, err error) {
	// fill empty parameters with defaults...
	if config.Port == 0 {
		config.Port = DEFAULT_SERVER_PORT
	}
	if len(config.LocalDomain) == 0 {
		config.LocalDomain = DEFAULT_LOCAL_DOMAIN
	}
	if len(config.UpstreamCfgFIle) == 0 {
		config.UpstreamCfgFIle = DEFAULT_CLI_CFG_FILE
	}
	if config.UpstreamCfg == nil {
		if config.UpstreamCfg, err = dns.ClientConfigFromFile(config.UpstreamCfgFIle); err != nil {
			return nil, err
		}
	}

	mdnsClient, err := NewMDNSClient()
	if err != nil {
		return
	}
	mdnsServer, err := NewMDNSServer(config.Zone)
	if err != nil {
		return
	}

	ifaceName := "default interface"
	if config.Iface != nil {
		ifaceName = config.Iface.Name
	}

	return &DNSServer{
		config:     &config,
		mdnsCli:    mdnsClient,
		mdnsSrv:    mdnsServer,
		IfaceName:  ifaceName,
		ListenAddr: fmt.Sprintf(":%d", config.Port),
	}, nil
}

// Start the DNS server
func (s *DNSServer) Start() error {
	Info.Printf("Using mDNS on %s", s.IfaceName)
	err := s.mdnsCli.Start(s.config.Iface)
	CheckFatal(err)
	err = s.mdnsSrv.Start(s.config.Iface, s.config.LocalDomain)
	CheckFatal(err)

	// create two DNS request multiplexerers, depending on the protocol used by clients
	// (we use the same protocol for asking upstream servers)
	mux := func(client *dns.Client) *dns.ServeMux {
		m := dns.NewServeMux()
		m.HandleFunc(s.config.LocalDomain, s.queryHandler([]Lookup{s.config.Zone, s.mdnsCli}))
		m.HandleFunc(RDNS_DOMAIN, s.rdnsHandler([]Lookup{s.config.Zone, s.mdnsCli}, client))
		m.HandleFunc(".", s.notUsHandler(client))
		return m
	}

	mdnsServer, err := NewMDNSServer(s.zone)
	CheckFatal(err)

	err = mdnsServer.Start(s.iface)
	CheckFatal(err)

	address := fmt.Sprintf(":%d", s.port)
	s.udpSrv = &dns.Server{Addr: address, Net: "udp", Handler: mux(&dns.Client{Net: "udp", UDPSize: UDPBufSize})}
	s.tcpSrv = &dns.Server{Addr: address, Net: "tcp", Handler: mux(&dns.Client{Net: "tcp"})}

	wg := new(sync.WaitGroup)
	wg.Add(2)

	go func() {
		defer wg.Done()

		Debug.Printf("Listening for DNS on %s (UDP)", address)
		err = s.udpSrv.ListenAndServe()
		CheckFatal(err)
		Debug.Printf("DNS UDP server exiting...")
	}()

	go func() {
		defer wg.Done()

		Debug.Printf("Listening for DNS on %s (TCP)", address)
		err = s.tcpSrv.ListenAndServe()
		CheckFatal(err)
		Debug.Printf("DNS TCP server exiting...")
	}()

	// Waiting for all goroutines to finish (otherwise they die as main routine dies)
	wg.Wait()

	Info.Printf("WeaveDNS server exiting...")
	return nil
}

// Perform a graceful shutdown
func (s *DNSServer) Stop() error {
	if err := s.tcpSrv.Shutdown(); err != nil {
		return err
	}
	if err := s.udpSrv.Shutdown(); err != nil {
		return err
	}
	// TODO: shutdown the mDNS client/server
	return nil
}

func (s *DNSServer) queryHandler(lookups []Lookup) dns.HandlerFunc {
	return func(w dns.ResponseWriter, r *dns.Msg) {
		q := r.Question[0]
		Debug.Printf("Query: %+v", q)
		if q.Qtype == dns.TypeA {
			for _, lookup := range lookups {
				if ip, err := lookup.LookupName(q.Name); err == nil {
					m := makeAddressReply(r, &q, []net.IP{ip})
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

func (s *DNSServer) rdnsHandler(lookups []Lookup, client *dns.Client) dns.HandlerFunc {
	fallback := s.notUsHandler(client)
	return func(w dns.ResponseWriter, r *dns.Msg) {
		q := r.Question[0]
		Debug.Printf("Reverse query: %+v", q)
		if q.Qtype == dns.TypePTR {
			for _, lookup := range lookups {
				if name, err := lookup.LookupInaddr(q.Name); err == nil {
					m := makePTRReply(r, &q, []string{name})
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
func (s *DNSServer) notUsHandler(dnsClient *dns.Client) dns.HandlerFunc {
	return func(w dns.ResponseWriter, r *dns.Msg) {
		q := r.Question[0]
		Debug.Printf("[dns msgid %d] Fallback query: %+v", r.MsgHdr.Id, q)
		for _, server := range s.config.UpstreamCfg.Servers {
			reply, _, err := dnsClient.Exchange(r, fmt.Sprintf("%s:%s", server, s.config.UpstreamCfg.Port))
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
