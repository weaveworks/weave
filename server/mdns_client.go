package weavedns

import (
	"github.com/miekg/dns"
	"log"
	"net"
)

// Portions of this code taken from github.com/armon/mdns

const (
	ipv4mdns = "224.0.0.251" // link-local multicast address
	mdnsPort = 5353          // mDNS assigned port
)

var (
	ipv4Addr = &net.UDPAddr{
		IP:   net.ParseIP(ipv4mdns),
		Port: mdnsPort,
	}
)

type ResponseA struct {
	Name string
	Addr net.IP
}

type responseInfo struct {
	ch chan<- *ResponseA
}

// Represents one query that we have sent for one name.
// If we, internally, get several requests for the same name while we have
// a query in flight, then we don't want to send more queries out.
type inflightQuery struct {
	name          string
	Id            uint16 // the DNS message ID
	responseInfos []*responseInfo
	// add timeout here ?
}

type MDNSClient struct {
	conn      *net.UDPConn
	addr      *net.UDPAddr
	inflight  map[string]*inflightQuery
	queryChan chan<- *MDNSInteraction
}

type mDNSQueryInfo struct {
	name       string
	querytype  uint16
	responseCh chan<- *ResponseA
}

func NewMDNSClient() (*MDNSClient, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		log.Println(err)
		return nil, err
	}
	retval := &MDNSClient{
		conn:     conn,
		addr:     ipv4Addr,
		inflight: make(map[string]*inflightQuery)}
	return retval, nil
}

func (c *MDNSClient) Start() {
	queryChan := make(chan *MDNSInteraction, 4)
	c.queryChan = queryChan
	go c.queryLoop(queryChan)
}

func LinkLocalMulticastListener(ifi *net.Interface) (net.PacketConn, error) {
	conn, err := net.ListenMulticastUDP("udp", ifi, ipv4Addr)
	return conn, err
}

func (c *MDNSClient) SendResponse(m *dns.Msg) error {
	buf, err := m.Pack()
	if err != nil {
		return err
	}
	_, err = c.conn.WriteTo(buf, c.addr)
	return err
}

// ACTOR client API

const (
	CSendQuery       = iota
	CShutdown        = iota
	CIsInflightQuery = iota
	CMessageReceived = iota
)

type Interaction struct {
	code       int
	resultChan chan<- interface{}
}

type MDNSInteraction struct {
	Interaction
	payload interface{}
}

// Async
func (c *MDNSClient) Shutdown() {
	c.queryChan <- &MDNSInteraction{Interaction: Interaction{code: CShutdown}}
}

// Async
func (c *MDNSClient) SendQuery(name string, querytype uint16, responseCh chan<- *ResponseA) {
	c.queryChan <- &MDNSInteraction{Interaction: Interaction{code: CSendQuery}, payload: mDNSQueryInfo{name, querytype, responseCh}}
}

// Sync
func (c *MDNSClient) IsInflightQuery(m *dns.Msg) bool {
	resultChan := make(chan interface{})
	c.queryChan <- &MDNSInteraction{Interaction: Interaction{code: CIsInflightQuery, resultChan: resultChan}, payload: m}
	result := <-resultChan
	return result.(bool)
}

// Async - called from dns library multiplexer
func (c *MDNSClient) ResponseCallback(r *dns.Msg) {
	c.queryChan <- &MDNSInteraction{Interaction: Interaction{code: CMessageReceived}, payload: r}
}

// ACTOR server

func (c *MDNSClient) queryLoop(queryChan <-chan *MDNSInteraction) {
	var err error
	terminate := false
	for !terminate {
		if err != nil {
			log.Printf("encountered error", err)
			break
		}
		query, ok := <-queryChan
		if !ok {
			break
		}
		switch query.code {
		case CShutdown:
			terminate = true
		case CSendQuery:
			err = c.handleSendQuery(query.payload.(mDNSQueryInfo))
		case CIsInflightQuery:
			query.resultChan <- c.handleIsInflightQuery(query.payload.(*dns.Msg))
		case CMessageReceived:
			c.handleResponse(query.payload.(*dns.Msg))
		}
	}
	// handle shutdown here
}

func (c *MDNSClient) handleSendQuery(q mDNSQueryInfo) error {
	m := new(dns.Msg)
	m.SetQuestion(q.name, q.querytype)
	m.RecursionDesired = false
	query, found := c.inflight[q.name]
	if !found {
		query = &inflightQuery{name: q.name, Id: m.Id}
		c.inflight[q.name] = query
	}
	info := &responseInfo{ch: q.responseCh}
	query.responseInfos = append(query.responseInfos, info)

	buf, err := m.Pack()
	if err != nil {
		return err
	}
	_, err = c.conn.WriteTo(buf, c.addr)
	return err
}

func (c *MDNSClient) handleIsInflightQuery(m *dns.Msg) bool {
	if len(m.Question) == 1 {
		q := m.Question[0]
		if q.Qtype == dns.TypeA {
			if query, found := c.inflight[q.Name]; found {
				if query.Id == m.Id {
					return true
				}
			}
		}
	}
	return false
}

func (c *MDNSClient) handleResponse(r *dns.Msg) {
	for _, answer := range r.Answer {
		switch rr := answer.(type) {
		case *dns.A:
			if query, found := c.inflight[rr.Hdr.Name]; found {
				for _, resp := range query.responseInfos {
					resp.ch <- &ResponseA{Name: rr.Hdr.Name, Addr: rr.A}
				}
				// To be simple for now, assume this is the only response coming
				delete(c.inflight, rr.Hdr.Name)
			} else {
				// We've received a response that didn't match a query
				// Do we want to cache it?
			}
		}
	}
}
