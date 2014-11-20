package nameserver

import (
	"github.com/miekg/dns"
	"math"
	"net"
	"time"
)

// Portions of this code taken from github.com/armon/mdns
// And portions from github.com/miekg/dns

const (
	ipv4mdns = "224.0.0.251" // link-local multicast address
	mdnsPort = 5353          // mDNS assigned port
	// We wait this long to hear a response from other mDNS servers on
	// the network.
	mDNSTimeout = 500 * time.Millisecond
	mDNSMaxSize = 1024 // Max size of response message
	MaxDuration = time.Duration(math.MaxInt64)
	MailboxSize = 16
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
	Err  error
}

type responseInfo struct {
	timeout time.Time // if no answer by this time, give up
	ch      chan<- *ResponseA
}

// Represents one query that we have sent for one name.
// If we, internally, get several requests for the same name while we have
// a query in flight, then we don't want to send more queries out.
// Invariant on responseInfos: they are in ascending order of timeout.
type inflightQuery struct {
	name          string
	id            uint16 // the DNS message ID
	responseInfos []*responseInfo
}

type MDNSClient struct {
	listener  *ResponseListener
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

type HandlerFunc func(*dns.Msg)

type ResponseListener struct {
	// For graceful shutdown.
	stopUDP chan bool
}

func (srv *ResponseListener) Activate(ifi *net.Interface, handler HandlerFunc) error {
	multicast, err := LinkLocalMulticastListener(ifi)
	if err != nil {
		return err
	}
	srv.stopUDP = make(chan bool)
	defer multicast.Close()
	for {
		m, e := srv.readUDP(multicast, 2*time.Second)
		select {
		case <-srv.stopUDP:
			return nil
		default:
		}
		if e != nil {
			continue
		}
		req := new(dns.Msg)
		if err := req.Unpack(m); err == nil {
			handler(req)
		} else {
			Warning.Printf("Error unpacking message %s", err)
		}
	}
}

func (srv *ResponseListener) Shutdown() {
	srv.stopUDP <- true
}

func (srv *ResponseListener) readUDP(conn *net.UDPConn, timeout time.Duration) ([]byte, error) {
	conn.SetReadDeadline(time.Now().Add(timeout))
	m := make([]byte, mDNSMaxSize)
	n, _, e := conn.ReadFrom(m)
	if e != nil || n == 0 {
		if e != nil {
			return nil, e
		}
		return nil, dns.ErrShortRead
	}
	m = m[:n]
	return m, nil
}

func NewMDNSClient() (*MDNSClient, error) {
	if conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0}); err != nil {
		return nil, err
	} else {
		return &MDNSClient{
			conn:     conn,
			addr:     ipv4Addr,
			inflight: make(map[string]*inflightQuery)}, nil
	}
}

func (c *MDNSClient) Start(ifi *net.Interface) error {
	handleMDNS := func(r *dns.Msg) {
		// Don't want to handle queries here, so filter anything out that isn't a response
		if len(r.Answer) > 0 {
			c.ResponseReceived(r)
		}
	}

	c.listener = &ResponseListener{}
	go c.listener.Activate(ifi, handleMDNS)

	queryChan := make(chan *MDNSInteraction, MailboxSize)
	c.queryChan = queryChan
	go c.queryLoop(queryChan)

	return nil
}

func LinkLocalMulticastListener(ifi *net.Interface) (*net.UDPConn, error) {
	return net.ListenMulticastUDP("udp", ifi, ipv4Addr)
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

// Async
func (c *MDNSClient) ResponseReceived(r *dns.Msg) {
	c.queryChan <- &MDNSInteraction{code: CMessageReceived, payload: r}
}

// ACTOR server

// Check all in-flight queries, close all that have already timed out,
// and return the duration until the next timeout
func (c *MDNSClient) checkInFlightQueries() time.Duration {
	now := time.Now()
	after := MaxDuration
	for name, query := range c.inflight {
		// Invariant on responseInfos: they are in ascending order of timeout.
		numClosed := 0
		for _, item := range query.responseInfos {
			duration := item.timeout.Sub(now)
			if duration <= 0 { // timed out
				close(item.ch)
				numClosed++
			} else {
				if duration < after {
					after = duration
				}
				break // don't need to look at any more for this query
			}
		}
		// Remove timed-out items from the slice
		query.responseInfos = query.responseInfos[numClosed:]
		if len(query.responseInfos) == 0 {
			delete(c.inflight, name)
		}
	}
	return after
}

func (c *MDNSClient) queryLoop(queryChan <-chan *MDNSInteraction) {
	timer := time.NewTimer(MaxDuration)
	run := func() {
		timer.Reset(c.checkInFlightQueries())
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
				c.listener.Shutdown()
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

	// Close all response channels at termination
	for _, query := range c.inflight {
		for _, item := range query.responseInfos {
			close(item.ch)
		}
	}
}

func (c *MDNSClient) handleSendQuery(q mDNSQueryInfo) {
	query, found := c.inflight[q.name]
	if !found {
		m := new(dns.Msg)
		m.SetQuestion(q.name, q.querytype)
		m.RecursionDesired = false
		buf, err := m.Pack()
		if err != nil {
			q.responseCh <- &ResponseA{Err: err}
			close(q.responseCh)
			return
		}
		query = &inflightQuery{
			name: q.name,
			id:   m.Id,
		}
		if _, err = c.conn.WriteTo(buf, c.addr); err != nil {
			q.responseCh <- &ResponseA{Err: err}
			close(q.responseCh)
			return
		}
		c.inflight[q.name] = query
	}
	info := &responseInfo{
		ch:      q.responseCh,
		timeout: time.Now().Add(mDNSTimeout),
	}
	// Invariant on responseInfos: they are in ascending order of timeout.
	// Since we use a fixed interval from Now(), this must be after all existing timeouts.
	query.responseInfos = append(query.responseInfos, info)
}

func (c *MDNSClient) handleResponse(r *dns.Msg) {
	for _, answer := range r.Answer {
		switch rr := answer.(type) {
		case *dns.A:
			name := rr.Hdr.Name
			if query, found := c.inflight[name]; found {
				for _, resp := range query.responseInfos {
					resp.ch <- &ResponseA{Name: rr.Hdr.Name, Addr: rr.A}
					close(resp.ch)
				}
				delete(c.inflight, name)
			} else {
				// We've received a response that didn't match a query
				// Do we want to cache it?
			}
		}
	}
}
