package nameserver

import (
	"fmt"
	"github.com/miekg/dns"
	. "github.com/zettio/weave/common"
	"net"
)

const (
	LOCAL_DOMAIN = "weave.local."
	UDPBufSize   = 4096 // bigger than the default 512
)

func makeDNSFailResponse(r *dns.Msg) *dns.Msg {
	m := new(dns.Msg)
	m.SetReply(r)
	m.RecursionAvailable = true
	m.Rcode = dns.RcodeNameError
	return m
}

type DNSServer struct {
	config *dns.ClientConfig
	zone   Zone
	iface  *net.Interface
	port   int
	udpSrv *dns.Server
	tcpSrv *dns.Server
}

// Creates a new DNS server with a given config
func NewDNSServer(config *dns.ClientConfig, zone Zone, iface *net.Interface, port int) *DNSServer {
	return &DNSServer{
		config: config,
		zone:   zone,
		iface:  iface,
		port:   port,
	}
}

// Start the DNS server
func (s *DNSServer) Start() error {
	mdnsClient, err := NewMDNSClient()
	checkFatal(err)

	ifaceName := "default interface"
	if s.iface != nil {
		ifaceName = s.iface.Name
	}
	Info.Printf("Using mDNS on %s", ifaceName)
	err = mdnsClient.Start(s.iface)
	checkFatal(err)

	// create two DNS request multiplexerers, depending on the protocol used by clients
	// (we use the same protocol for asking upstream servers)
	mux := func(client *dns.Client) *dns.ServeMux {
		m := dns.NewServeMux()
		m.HandleFunc(LOCAL_DOMAIN, s.queryHandler([]Lookup{s.zone, mdnsClient}))
		m.HandleFunc(RDNS_DOMAIN, s.rdnsHandler([]Lookup{s.zone, mdnsClient}, client))
		m.HandleFunc(".", s.notUsHandler(client))
		return m
	}

	mdnsServer, err := NewMDNSServer(s.zone)
	checkFatal(err)

	err = mdnsServer.Start(s.iface)
	checkFatal(err)

	address := fmt.Sprintf(":%d", s.port)
	s.udpSrv = &dns.Server{Addr: address, Net: "udp", Handler: mux(&dns.Client{Net: "udp", UDPSize: UDPBufSize})}
	s.tcpSrv = &dns.Server{Addr: address, Net: "tcp", Handler: mux(&dns.Client{Net: "tcp"})}

	go func() {
		Info.Printf("Listening for DNS on %s (UDP)", address)
		err = s.udpSrv.ListenAndServe()
		checkFatal(err)
	}()

	go func() {
		Info.Printf("Listening for DNS on %s (TCP)", address)
		err = s.tcpSrv.ListenAndServe()
		checkFatal(err)
	}()

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
		for _, server := range s.config.Servers {
			reply, _, err := dnsClient.Exchange(r, fmt.Sprintf("%s:%s", server, s.config.Port))
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
