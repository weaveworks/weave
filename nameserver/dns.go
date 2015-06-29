package nameserver

import (
	"bytes"
	"fmt"
	"math/rand"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/miekg/dns"

	"github.com/weaveworks/weave/net/address"
)

const (
	topDomain        = "."
	reverseDNSdomain = "in-addr.arpa."
	etcResolvConf    = "/etc/resolv.conf"
	udpBuffSize      = uint16(4096)
	minUDPSize       = 512

	DefaultPort          = 53
	DefaultTTL           = 1
	DefaultClientTimeout = 5 * time.Second
)

type DNSServer struct {
	ns     *Nameserver
	domain string
	ttl    uint32
	port   int

	servers   []*dns.Server
	upstream  *dns.ClientConfig
	tcpClient *dns.Client
	udpClient *dns.Client
}

func NewDNSServer(ns *Nameserver, domain string, port int, ttl uint32,
	clientTimeout time.Duration) (*DNSServer, error) {

	s := &DNSServer{
		ns:        ns,
		domain:    dns.Fqdn(domain),
		ttl:       ttl,
		port:      port,
		tcpClient: &dns.Client{Net: "tcp", ReadTimeout: clientTimeout},
		udpClient: &dns.Client{Net: "udp", ReadTimeout: clientTimeout, UDPSize: udpBuffSize},
	}
	var err error
	if s.upstream, err = dns.ClientConfigFromFile(etcResolvConf); err != nil {
		return nil, err
	}

	err = s.listen(port)
	return s, err
}

func (d *DNSServer) String() string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "WeaveDNS (%s)\n", d.ns.ourName)
	fmt.Fprintf(&buf, "  listening on port %d, for domain %s\n", d.port, d.domain)
	fmt.Fprintf(&buf, "  response ttl %d\n", d.ttl)
	return buf.String()
}

func (d *DNSServer) listen(port int) error {
	udpListener, err := net.ListenPacket("udp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	udpServer := &dns.Server{PacketConn: udpListener, Handler: d.createMux(d.udpClient, minUDPSize)}

	tcpListener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		udpServer.Shutdown()
		return err
	}
	tcpServer := &dns.Server{Listener: tcpListener, Handler: d.createMux(d.tcpClient, -1)}

	d.servers = []*dns.Server{udpServer, tcpServer}
	return nil
}

func (d *DNSServer) ActivateAndServe() {
	for _, server := range d.servers {
		go func(server *dns.Server) {
			server.ActivateAndServe()
		}(server)
	}
}

func (d *DNSServer) Stop() error {
	for _, server := range d.servers {
		if err := server.Shutdown(); err != nil {
			return err
		}
	}
	return nil
}

func (d *DNSServer) errorResponse(r *dns.Msg, code int, w dns.ResponseWriter) {
	m := dns.Msg{}
	m.SetReply(r)
	m.RecursionAvailable = true
	m.Rcode = code

	d.ns.debugf("error response: %+v", m)
	if err := w.WriteMsg(&m); err != nil {
		d.ns.infof("error responding: %v", err)
	}
}

func (d *DNSServer) createMux(client *dns.Client, defaultMaxResponseSize int) *dns.ServeMux {
	m := dns.NewServeMux()
	m.HandleFunc(d.domain, d.handleLocal(defaultMaxResponseSize))
	m.HandleFunc(reverseDNSdomain, d.handleReverse(client, defaultMaxResponseSize))
	m.HandleFunc(topDomain, d.handleRecursive(client, defaultMaxResponseSize))
	return m
}

func (d *DNSServer) handleLocal(defaultMaxResponseSize int) func(dns.ResponseWriter, *dns.Msg) {
	return func(w dns.ResponseWriter, req *dns.Msg) {
		d.ns.debugf("local request: %+v", *req)
		if len(req.Question) != 1 || req.Question[0].Qtype != dns.TypeA {
			d.errorResponse(req, dns.RcodeNameError, w)
			return
		}

		hostname := req.Question[0].Name
		addrs := d.ns.Lookup(hostname)

		response := dns.Msg{}
		response.RecursionAvailable = true
		response.Authoritative = true
		response.SetReply(req)
		response.Answer = make([]dns.RR, len(addrs))

		header := dns.RR_Header{
			Name:   hostname,
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    d.ttl,
		}

		for i, addr := range addrs {
			ip := addr.IP4()
			response.Answer[i] = &dns.A{Hdr: header, A: ip}
		}

		shuffleAnswers(&response.Answer)
		maxResponseSize := getMaxResponseSize(req, defaultMaxResponseSize)
		truncateResponse(&response, maxResponseSize)

		d.ns.debugf("response: %+v", response)
		if err := w.WriteMsg(&response); err != nil {
			d.ns.infof("error responding: %v", err)
		}
	}
}

func (d *DNSServer) handleReverse(client *dns.Client, defaultMaxResponseSize int) func(dns.ResponseWriter, *dns.Msg) {
	return func(w dns.ResponseWriter, req *dns.Msg) {
		d.ns.debugf("reverse request: %+v", *req)
		if len(req.Question) != 1 || req.Question[0].Qtype != dns.TypePTR {
			d.errorResponse(req, dns.RcodeNameError, w)
			return
		}

		ipStr := strings.TrimSuffix(req.Question[0].Name, "."+reverseDNSdomain)
		ip, err := address.ParseIP(ipStr)
		if err != nil {
			d.errorResponse(req, dns.RcodeNameError, w)
			return
		}

		hostname, err := d.ns.ReverseLookup(ip.Reverse())
		if err != nil {
			d.handleRecursive(client, defaultMaxResponseSize)(w, req)
			return
		}

		response := dns.Msg{}
		response.RecursionAvailable = true
		response.Authoritative = true
		response.SetReply(req)

		header := dns.RR_Header{
			Name:   req.Question[0].Name,
			Rrtype: dns.TypePTR,
			Class:  dns.ClassINET,
			Ttl:    d.ttl,
		}

		response.Answer = []dns.RR{&dns.PTR{
			Hdr: header,
			Ptr: hostname,
		}}

		maxResponseSize := getMaxResponseSize(req, defaultMaxResponseSize)
		truncateResponse(&response, maxResponseSize)

		d.ns.debugf("response: %+v", response)
		if err := w.WriteMsg(&response); err != nil {
			d.ns.infof("error responding: %v", err)
		}
	}
}

func (d *DNSServer) handleRecursive(client *dns.Client, defaultMaxResponseSize int) func(dns.ResponseWriter, *dns.Msg) {
	return func(w dns.ResponseWriter, req *dns.Msg) {
		d.ns.debugf("recursive cdrequest: %+v", *req)
		for _, server := range d.upstream.Servers {
			response, _, err := client.Exchange(req, fmt.Sprintf("%s:%s", server, d.upstream.Port))
			if err != nil || response == nil {
				d.ns.debugf("network error trying %s (%s)", server, err)
				continue
			}
			if response.Rcode != dns.RcodeSuccess && !response.Authoritative {
				d.ns.debugf("network error trying %s (%s)", server, err)
				continue
			}
			d.ns.debugf("response: %+v", response)
			if err := w.WriteMsg(response); err != nil {
				d.ns.infof("error responding: %v", err)
			}
			return
		}

		d.errorResponse(req, dns.RcodeServerFailure, w)
	}
}

func shuffleAnswers(answers *[]dns.RR) {
	if len(*answers) <= 1 {
		return
	}

	for i := range *answers {
		j := rand.Intn(i + 1)
		(*answers)[i], (*answers)[j] = (*answers)[j], (*answers)[i]
	}
}

func truncateResponse(response *dns.Msg, maxSize int) {
	if len(response.Answer) <= 1 || maxSize <= 0 {
		return
	}

	// take a copy of answers, as we're going to mutate response
	answers := response.Answer

	// search for smallest i that is too big
	i := sort.Search(len(response.Answer), func(i int) bool {
		// return true if too big
		response.Answer = answers[:i+1]
		return response.Len() > maxSize
	})
	if i == len(answers) {
		response.Answer = answers
		return
	}

	response.Answer = answers[:i]
	response.Truncated = true
}

func getMaxResponseSize(req *dns.Msg, defaultMaxResponseSize int) int {
	if opt := req.IsEdns0(); opt != nil {
		return int(opt.UDPSize())
	}
	return defaultMaxResponseSize
}
