package weavedns

import (
	"github.com/miekg/dns"
	"log"
	"net"
	"sync"
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
	sync.RWMutex
	conn     *net.UDPConn
	addr     *net.UDPAddr
	inflight map[string]*inflightQuery
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

func LinkLocalMulticastListener() (net.PacketConn, error) {
	conn, err := net.ListenMulticastUDP("udp", nil, ipv4Addr)
	return conn, err
}

func (c *MDNSClient) SendQuery(name string, querytype uint16, responseCh chan<- *ResponseA) error {
	m := new(dns.Msg)
	m.SetQuestion(name, querytype)
	m.RecursionDesired = false

	c.Lock()
	query, found := c.inflight[name]
	if !found {
		query = &inflightQuery{name: name}
		c.inflight[name] = query
	}
	info := &responseInfo{ch: responseCh}
	query.responseInfos = append(query.responseInfos, info)
	c.Unlock()

	buf, err := m.Pack()
	if err != nil {
		return err
	}
	_, err = c.conn.WriteTo(buf, c.addr)
	return err
}

func (c *MDNSClient) SendResponse(m *dns.Msg) error {
	buf, err := m.Pack()
	if err != nil {
		return err
	}
	_, err = c.conn.WriteTo(buf, c.addr)
	return err
}

func (c *MDNSClient) HandleResponse(r *dns.Msg) {
	for _, answer := range r.Answer {
		switch rr := answer.(type) {
		case *dns.A:
			c.Lock()
			if query, found := c.inflight[rr.Hdr.Name]; found {
				for _, resp := range query.responseInfos {
					resp.ch <- &ResponseA{Name: rr.Hdr.Name, Addr: rr.A}
				}
				// To be simple for now, assume this is the only response coming
				delete(c.inflight, rr.Hdr.Name)
			} else {
				log.Println("Response received that didn't match query", r)
			}
			c.Unlock()
		}
	}
}
