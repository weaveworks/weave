package weavedns

import (
	"github.com/miekg/dns"
	"math"
	"net"
	"time"
)

// Portions of this code taken from github.com/armon/mdns

const (
	ipv4mdns    = "224.0.0.251" // link-local multicast address
	mdnsPort    = 5353          // mDNS assigned port
	mDNSTimeout = time.Second
	MaxDuration = time.Duration(math.MaxInt64)
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
	err  error
}

type responseInfo struct {
	timeout time.Time // if no answer by this time, give up
	ch      chan<- *ResponseA
}

// Represents one query that we have sent for one name.
// If we, internally, get several requests for the same name while we have
// a query in flight, then we don't want to send more queries out.
type inflightQuery struct {
	name          string
	Id            uint16 // the DNS message ID
	responseInfos []*responseInfo
}

type MDNSClient struct {
	server    *dns.Server
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
		return nil, err
	}
	retval := &MDNSClient{
		conn:     conn,
		addr:     ipv4Addr,
		inflight: make(map[string]*inflightQuery)}
	return retval, nil
}

func (c *MDNSClient) Start(ifi *net.Interface) error {
	multicast, err := LinkLocalMulticastListener(ifi)
	if err != nil {
		return err
	}

	handleMDNS := func(w dns.ResponseWriter, r *dns.Msg) {
		//log.Println("client received:", r)
		// Only handle responses here
		if len(r.Answer) > 0 {
			c.ResponseCallback(r)
		}
	}

	c.server = &dns.Server{Listener: nil, PacketConn: multicast, Handler: dns.HandlerFunc(handleMDNS)}
	go c.server.ActivateAndServe()

	queryChan := make(chan *MDNSInteraction, 4)
	c.queryChan = queryChan
	go c.queryLoop(queryChan)

	return nil
}

func LinkLocalMulticastListener(ifi *net.Interface) (net.PacketConn, error) {
	conn, err := net.ListenMulticastUDP("udp", ifi, ipv4Addr)
	return conn, err
}

// ACTOR client API

const (
	CSendQuery       = iota
	CShutdown        = iota
	CMessageReceived = iota
)

type MDNSInteraction struct {
	code       int
	resultChan chan<- interface{}
	payload    interface{}
}

// Async
func (c *MDNSClient) Shutdown() {
	c.queryChan <- &MDNSInteraction{code: CShutdown}
}

// Async
func (c *MDNSClient) SendQuery(name string, querytype uint16, responseCh chan<- *ResponseA) {
	c.queryChan <- &MDNSInteraction{
		code:    CSendQuery,
		payload: mDNSQueryInfo{name, querytype, responseCh},
	}
}

// Async - called from dns library multiplexer
func (c *MDNSClient) ResponseCallback(r *dns.Msg) {
	c.queryChan <- &MDNSInteraction{code: CMessageReceived, payload: r}
}

// ACTOR server

func (c *MDNSClient) queryLoop(queryChan <-chan *MDNSInteraction) {
	timer := time.NewTimer(MaxDuration)
	run := func() {
		now := time.Now()
		after := MaxDuration
		for name, query := range c.inflight {
			// Count down from end of slice to beginning
			length := len(query.responseInfos)
			for i := length - 1; i >= 0; i-- {
				item := query.responseInfos[i]
				switch duration := item.timeout.Sub(now); {
				case duration <= 0: // timed out
					close(item.ch)
					// Swap item from the end of the slice
					length--
					if i < length {
						query.responseInfos[i] = query.responseInfos[length]
					}
				case duration < after:
					after = duration
				}
			}
			query.responseInfos = query.responseInfos[:length]
			if length == 0 {
				delete(c.inflight, name)
			}
		}
		timer.Reset(after)
	}

	terminate := false
	for !terminate {
		select {
		case query, ok := <-queryChan:
			if !ok {
				break
			}
			switch query.code {
			case CShutdown:
				c.server.Shutdown()
				terminate = true
			case CSendQuery:
				c.handleSendQuery(query.payload.(mDNSQueryInfo))
				run()
			case CMessageReceived:
				c.handleResponse(query.payload.(*dns.Msg))
				run()
			}
		case <-timer.C:
			run()
		}
	}

	// Close all response channels
	for _, query := range c.inflight {
		for _, item := range query.responseInfos {
			close(item.ch)
		}
	}
}

func (c *MDNSClient) handleSendQuery(q mDNSQueryInfo) error {
	query, found := c.inflight[q.name]
	if !found {
		m := new(dns.Msg)
		m.SetQuestion(q.name, q.querytype)
		m.RecursionDesired = false
		buf, err := m.Pack()
		if err != nil {
			q.responseCh <- &ResponseA{err: err}
			close(q.responseCh)
			return err
		}
		query = &inflightQuery{
			name: q.name,
			Id:   m.Id,
		}
		c.inflight[q.name] = query
		_, err = c.conn.WriteTo(buf, c.addr)
		if err != nil {
			q.responseCh <- &ResponseA{err: err}
			close(q.responseCh)
			return err
		}
	}
	info := &responseInfo{
		ch:      q.responseCh,
		timeout: time.Now().Add(mDNSTimeout),
	}
	query.responseInfos = append(query.responseInfos, info)

	return nil
}

func (c *MDNSClient) handleResponse(r *dns.Msg) {
	for _, answer := range r.Answer {
		switch rr := answer.(type) {
		case *dns.A:
			if query, found := c.inflight[rr.Hdr.Name]; found {
				for _, resp := range query.responseInfos {
					resp.ch <- &ResponseA{Name: rr.Hdr.Name, Addr: rr.A}
				}
			} else {
				// We've received a response that didn't match a query
				// Do we want to cache it?
			}
		}
	}
}
