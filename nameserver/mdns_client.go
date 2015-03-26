package nameserver

import (
	"github.com/miekg/dns"
	"math"
	"net"
	"time"
)

// Portions of this code taken from github.com/armon/mdns

const (
	ipv4mdns = "224.0.0.251" // link-local multicast address
	mdnsPort = 5353          // mDNS assigned port
	// We wait this long to hear a response from other mDNS servers on
	// the network.
	mDNSTimeout = 500 * time.Millisecond
	MaxDuration = time.Duration(math.MaxInt64)
	MailboxSize = 16
)

var (
	ipv4Addr = &net.UDPAddr{
		IP:   net.ParseIP(ipv4mdns),
		Port: mdnsPort,
	}
)

type Response struct {
	Name string
	Addr net.IP
	Err  error
}

type responseInfo struct {
	timeout time.Time // if no answer by this time, give up
	ch      chan<- *Response
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
	// note unorthodox use of 'Server' class in client logic - using it to
	// listen on a multicast socket and call callback whenever a message comes in.
	listener   *dns.Server
	conn       *net.UDPConn
	addr       *net.UDPAddr
	inflight   map[string]*inflightQuery
	actionChan chan<- MDNSAction
}

type mDNSQueryInfo struct {
	name       string
	querytype  uint16
	responseCh chan<- *Response
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
	multicast, err := LinkLocalMulticastListener(ifi)
	if err != nil {
		return err
	}

	handleMDNS := func(w dns.ResponseWriter, r *dns.Msg) {
		// Don't want to handle queries here, so filter anything out that isn't a response
		if len(r.Answer) > 0 {
			c.ResponseCallback(r)
		}
	}

	c.listener = &dns.Server{Unsafe: true, PacketConn: multicast, Handler: dns.HandlerFunc(handleMDNS)}
	go c.listener.ActivateAndServe()

	actionChan := make(chan MDNSAction, MailboxSize)
	c.actionChan = actionChan
	go c.actorLoop(actionChan)

	return nil
}

func LinkLocalMulticastListener(ifi *net.Interface) (net.PacketConn, error) {
	return net.ListenMulticastUDP("udp", ifi, ipv4Addr)
}

// ACTOR client API

type MDNSAction func()

// Async
func (c *MDNSClient) SendQuery(name string, querytype uint16, responseCh chan<- *Response) {
	c.actionChan <- func() {
		query, found := c.inflight[name]
		if !found {
			m := new(dns.Msg)
			m.SetQuestion(name, querytype)
			m.RecursionDesired = false
			buf, err := m.Pack()
			if err != nil {
				responseCh <- &Response{Err: err}
				close(responseCh)
				return
			}
			query = &inflightQuery{
				name: name,
				id:   m.Id,
			}
			if _, err = c.conn.WriteTo(buf, c.addr); err != nil {
				responseCh <- &Response{Err: err}
				close(responseCh)
				return
			}
			c.inflight[name] = query
		}
		info := &responseInfo{
			ch:      responseCh,
			timeout: time.Now().Add(mDNSTimeout),
		}
		// Invariant on responseInfos: they are in ascending order of
		// timeout.  Since we use a fixed interval from Now(), this
		// must be after all existing timeouts.
		query.responseInfos = append(query.responseInfos, info)
	}
}

// Async - called from dns library multiplexer
func (c *MDNSClient) ResponseCallback(r *dns.Msg) {
	c.actionChan <- func() {
		for _, answer := range r.Answer {
			var name string
			var res *Response

			switch rr := answer.(type) {
			case *dns.A:
				name = rr.Hdr.Name
				res = &Response{Addr: rr.A}
			case *dns.PTR:
				name = rr.Hdr.Name
				res = &Response{Name: rr.Ptr}
			default:
				return
			}

			if query, found := c.inflight[name]; found {
				for _, resp := range query.responseInfos {
					resp.ch <- res
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

func (c *MDNSClient) actorLoop(actionChan <-chan MDNSAction) {
	timer := time.NewTimer(MaxDuration)
	run := func() { timer.Reset(c.checkInFlightQueries()) }
	terminate := false
	for !terminate {
		select {
		case action := <-actionChan:
			if action == nil {
				c.listener.Shutdown()
				terminate = true
			} else {
				action()
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
