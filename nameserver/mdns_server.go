package nameserver

import (
	"net"

	"github.com/miekg/dns"
	. "github.com/weaveworks/weave/common"
)

type MDNSServer struct {
	localAddrs []net.Addr
	sendconn   *net.UDPConn
	srv        *dns.Server
	zone       Zone
	allowLocal bool
	ttl        int
}

// Create a new mDNS server
// Nothing will be done (including port bindings) until you `Start()` the server
func NewMDNSServer(zone Zone, local bool, ttl int) (*MDNSServer, error) {
	return &MDNSServer{
		zone:       zone,
		allowLocal: local,
		ttl:        ttl,
	}, nil
}

// Return true if testaddr is a UDP address with IP matching my local i/f
func (s *MDNSServer) addrIsLocal(testaddr net.Addr) bool {
	if udpaddr, ok := testaddr.(*net.UDPAddr); ok {
		for _, localaddr := range s.localAddrs {
			if ipnetlocal, ok := localaddr.(*net.IPNet); ok {
				if ipnetlocal.IP.Equal(udpaddr.IP) {
					return true
				}
			}
		}
	}
	return false
}

// Start the mDNS server
func (s *MDNSServer) Start(ifi *net.Interface) (err error) {
	// This is a bit of a kludge - per the RFC we should send responses from 5353, but that doesn't seem to work
	s.sendconn, err = net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return err
	}

	conn, err := LinkLocalMulticastListener(ifi)
	if err != nil {
		return err
	}

	if ifi == nil {
		s.localAddrs, err = net.InterfaceAddrs()
	} else {
		s.localAddrs, err = ifi.Addrs()
	}
	if err != nil {
		return err
	}

	handleLocal := s.makeHandler(dns.TypeA,
		func(zone ZoneLookup, r *dns.Msg, q *dns.Question) *dns.Msg {
			if ips, err := zone.LookupName(q.Name); err == nil {
				return makeAddressReply(r, q, ips, s.ttl)
			}
			return nil
		})

	handleReverse := s.makeHandler(dns.TypePTR,
		func(zone ZoneLookup, r *dns.Msg, q *dns.Question) *dns.Msg {
			if names, err := zone.LookupInaddr(q.Name); err == nil {
				return makePTRReply(r, q, names, s.ttl)
			}
			return nil
		})

	mux := dns.NewServeMux()
	mux.HandleFunc(s.zone.Domain(), handleLocal)
	mux.HandleFunc(RDNSDomain, handleReverse)

	s.srv = &dns.Server{
		Listener:   nil,
		PacketConn: conn,
		Handler:    mux,
	}
	go s.srv.ActivateAndServe()
	return err
}

// Stop the mDNS server
func (s *MDNSServer) Stop() error {
	return s.srv.Shutdown()
}

func (s *MDNSServer) Zone() Zone {
	return s.zone
}

type LookupFunc func(ZoneLookup, *dns.Msg, *dns.Question) *dns.Msg

func (s *MDNSServer) makeHandler(qtype uint16, lookup LookupFunc) dns.HandlerFunc {
	return func(rw dns.ResponseWriter, r *dns.Msg) {
		// Handle only questions, ignore answers. We might also ignore
		// questions that arise locally (i.e., that come from an IP we
		// think is local), but in the interest of avoiding
		// complication, and easier testing, this is elided on the
		// assumption that the client wouldn't ask if it already knew
		// the answer, and if it does ask, it'll be happy to get an
		// answer.
		remoteAddr := rw.RemoteAddr()
		if s.addrIsLocal(remoteAddr) && !s.allowLocal {
			Log.Debugf("[mdns] srv: mDNS query from local host ('%s'): ignored", remoteAddr)
			return
		}

		// TODO: use any Answer in the query for adding records in the Zone database...
		if len(r.Answer) == 0 && len(r.Question) > 0 {
			q := &r.Question[0]
			if q.Qtype == qtype {
				Log.Debugf("[mdns msgid %d] srv: trying to answer to mDNS query '%s'", r.MsgHdr.Id, q.Name)
				if m := lookup(s.zone, r, q); m != nil {
					Log.Debugf("[mdns msgid %d] srv: found local answer to mDNS query '%s'", r.MsgHdr.Id, q.Name)
					if err := s.sendResponse(m); err != nil {
						Log.Warningf("[mdns msgid %d] srv: error writing mDNS response to %v", r.MsgHdr.Id, s.sendconn)
					} else {
						Log.Debugf("[mdns msgid %d] srv: response sent: %d answers", r.MsgHdr.Id, len(m.Answer))
					}
				} else {
					Log.Debugf("[mdns msgid %d] srv: no local answer for answering mDNS query '%s'",
						r.MsgHdr.Id, q.Name)
				}
			}
		}
	}
}

func (s *MDNSServer) sendResponse(m *dns.Msg) error {
	buf, err := m.Pack()
	if err != nil {
		return err
	}
	_, err = s.sendconn.WriteTo(buf, ipv4Addr)
	return err
}
