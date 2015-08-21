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

	DefaultListenAddress = "0.0.0.0:53"
	DefaultTTL           = 1
	DefaultClientTimeout = 5 * time.Second
)

type DNSServer struct {
	ns      *Nameserver
	domain  string
	ttl     uint32
	address string

	servers   []*dns.Server
	upstream  *dns.ClientConfig
	tcpClient *dns.Client
	udpClient *dns.Client
}

func NewDNSServer(ns *Nameserver, domain string, address string, ttl uint32, clientTimeout time.Duration) (*DNSServer, error) {

	s := &DNSServer{
		ns:        ns,
		domain:    dns.Fqdn(domain),
		ttl:       ttl,
		address:   address,
		tcpClient: &dns.Client{Net: "tcp", ReadTimeout: clientTimeout},
		udpClient: &dns.Client{Net: "udp", ReadTimeout: clientTimeout, UDPSize: udpBuffSize},
	}
	var err error
	if s.upstream, err = dns.ClientConfigFromFile(etcResolvConf); err != nil {
		return nil, err
	}

	err = s.listen(address)
	return s, err
}

func (d *DNSServer) String() string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "WeaveDNS (%s)\n", d.ns.ourName)
	fmt.Fprintf(&buf, "  listening on %s, for domain %s\n", d.address, d.domain)
	fmt.Fprintf(&buf, "  response ttl %d\n", d.ttl)
	return buf.String()
}

func (d *DNSServer) listen(address string) error {
	udpListener, err := net.ListenPacket("udp", address)
	if err != nil {
		return err
	}
	udpServer := &dns.Server{PacketConn: udpListener, Handler: d.createMux(d.udpClient, minUDPSize)}

	tcpListener, err := net.Listen("tcp", address)
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

type handler struct {
	*DNSServer
	maxResponseSize int
	client          *dns.Client
}

func (d *DNSServer) createMux(client *dns.Client, defaultMaxResponseSize int) *dns.ServeMux {
	m := dns.NewServeMux()
	h := &handler{
		DNSServer:       d,
		maxResponseSize: defaultMaxResponseSize,
		client:          client,
	}
	m.HandleFunc(d.domain, h.handleLocal)
	m.HandleFunc(reverseDNSdomain, h.handleReverse)
	m.HandleFunc(topDomain, h.handleRecursive)
	return m
}

func (h *handler) handleLocal(w dns.ResponseWriter, req *dns.Msg) {
	h.ns.debugf("local request: %+v", *req)
	if len(req.Question) != 1 || req.Question[0].Qtype != dns.TypeA {
		h.nameError(w, req)
		return
	}

	hostname := dns.Fqdn(req.Question[0].Name)
	if strings.Count(hostname, ".") == 1 {
		hostname = hostname + h.domain
	}

	addrs := h.ns.Lookup(hostname)
	if len(addrs) == 0 {
		h.nameError(w, req)
		return
	}

	header := dns.RR_Header{
		Name:   req.Question[0].Name,
		Rrtype: dns.TypeA,
		Class:  dns.ClassINET,
		Ttl:    h.ttl,
	}
	answers := make([]dns.RR, len(addrs))
	for i, addr := range addrs {
		ip := addr.IP4()
		answers[i] = &dns.A{Hdr: header, A: ip}
	}
	shuffleAnswers(&answers)

	h.respond(w, h.makeResponse(req, answers))
}

func (h *handler) handleReverse(w dns.ResponseWriter, req *dns.Msg) {
	h.ns.debugf("reverse request: %+v", *req)
	if len(req.Question) != 1 || req.Question[0].Qtype != dns.TypePTR {
		h.nameError(w, req)
		return
	}

	ipStr := strings.TrimSuffix(req.Question[0].Name, "."+reverseDNSdomain)
	ip, err := address.ParseIP(ipStr)
	if err != nil {
		h.nameError(w, req)
		return
	}

	hostname, err := h.ns.ReverseLookup(ip.Reverse())
	if err != nil {
		h.handleRecursive(w, req)
		return
	}

	header := dns.RR_Header{
		Name:   req.Question[0].Name,
		Rrtype: dns.TypePTR,
		Class:  dns.ClassINET,
		Ttl:    h.ttl,
	}
	answers := []dns.RR{&dns.PTR{
		Hdr: header,
		Ptr: hostname,
	}}

	h.respond(w, h.makeResponse(req, answers))
}

func (h *handler) handleRecursive(w dns.ResponseWriter, req *dns.Msg) {
	h.ns.debugf("recursive request: %+v", *req)

	// Resolve unqualified names locally
	if len(req.Question) == 1 && req.Question[0].Qtype == dns.TypeA {
		hostname := dns.Fqdn(req.Question[0].Name)
		if strings.Count(hostname, ".") == 1 {
			h.handleLocal(w, req)
			return
		}
	}

	for _, server := range h.upstream.Servers {
		reqCopy := req.Copy()
		reqCopy.Id = dns.Id()
		response, _, err := h.client.Exchange(reqCopy, fmt.Sprintf("%s:%s", server, h.upstream.Port))
		if err != nil || response == nil {
			h.ns.debugf("error trying %s: %v", server, err)
			continue
		}
		response.Id = req.Id
		if h.responseTooBig(req, response) {
			response.Compress = true
		}
		h.respond(w, response)
		return
	}

	h.respond(w, h.makeErrorResponse(req, dns.RcodeServerFailure))
}

func (h *handler) makeResponse(req *dns.Msg, answers []dns.RR) *dns.Msg {
	response := &dns.Msg{}
	response.SetReply(req)
	response.RecursionAvailable = true
	response.Authoritative = true
	response.Answer = answers
	if !h.responseTooBig(req, response) {
		return response
	}

	// search for smallest i that is too big
	maxSize := h.getMaxResponseSize(req)
	i := sort.Search(len(answers), func(i int) bool {
		// return true if too big
		response.Answer = answers[:i+1]
		return response.Len() > maxSize
	})

	response.Answer = answers[:i]
	if i < len(answers) {
		response.Truncated = true
	}
	return response
}

func (h *handler) makeErrorResponse(req *dns.Msg, code int) *dns.Msg {
	response := &dns.Msg{}
	response.SetReply(req)
	response.RecursionAvailable = true
	response.Rcode = code
	return response
}

func (h *handler) responseTooBig(req, response *dns.Msg) bool {
	return len(response.Answer) > 1 && h.maxResponseSize > 0 && response.Len() > h.getMaxResponseSize(req)
}

func (h *handler) respond(w dns.ResponseWriter, response *dns.Msg) {
	h.ns.debugf("response: %+v", response)
	if err := w.WriteMsg(response); err != nil {
		h.ns.infof("error responding: %v", err)
	}
}

func (h *handler) nameError(w dns.ResponseWriter, req *dns.Msg) {
	h.respond(w, h.makeErrorResponse(req, dns.RcodeNameError))
}

func (h *handler) getMaxResponseSize(req *dns.Msg) int {
	if opt := req.IsEdns0(); opt != nil {
		return int(opt.UDPSize())
	}
	return h.maxResponseSize
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
