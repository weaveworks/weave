package nameserver

import (
	"github.com/miekg/dns"
	. "github.com/zettio/weave/common"
	"net"
)

type MDNSServer struct {
	localAddrs []net.Addr
	sendconn   *net.UDPConn
	zone       Zone
}

func NewMDNSServer(zone Zone) (*MDNSServer, error) {
	// This is a bit of a kludge - per the RFC we should send responses from 5353, but that doesn't seem to work
	sendconn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, err
	}
	return &MDNSServer{sendconn: sendconn, zone: zone}, nil
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

func (s *MDNSServer) Start(ifi *net.Interface) error {
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

	handler := func(qtype uint16, lookup func(*dns.Msg, *dns.Question) *dns.Msg) dns.HandlerFunc {
		return func(w dns.ResponseWriter, r *dns.Msg) {
			// Handle only questions, ignore answers. We might also
			// ignore questions that arise locally (i.e., that come
			// from an IP we think is local), but in the interest of
			// avoiding complication, and easier testing, this is
			// elided on the assumption that the client wouldn't ask
			// if it already knew the answer, and if it does ask,
			// it'll be happy to get an answer.
			if len(r.Answer) == 0 && len(r.Question) > 0 {
				q := &r.Question[0]
				if q.Qtype == qtype {
					if m := lookup(r, q); m != nil {
						if err = s.sendResponse(m); err != nil {
							Warning.Printf("Error writing to %s", w)
						}
					} else {
						Debug.Printf("No local answer for mDNS query %s", q.Name)
					}
				}
			}
		}
	}

	handleLocal := handler(dns.TypeA, func(r *dns.Msg, q *dns.Question) *dns.Msg {
		if ip, err := s.zone.LookupLocal(q.Name); err == nil {
			return makeAddressReply(r, q, []net.IP{ip})
		} else {
			return nil
		}
	})

	handleReverse := handler(dns.TypePTR, func(r *dns.Msg, q *dns.Question) *dns.Msg {
		if name, err := s.zone.ReverseLookupLocal(q.Name); err == nil {
			return makePTRReply(r, q, []string{name})
		} else {
			return nil
		}
	})

	mux := dns.NewServeMux()
	mux.HandleFunc(LOCAL_DOMAIN, handleLocal)
	mux.HandleFunc(RDNS_DOMAIN, handleReverse)

	go dns.ActivateAndServe(nil, conn, mux)
	return err
}

func (s *MDNSServer) sendResponse(m *dns.Msg) error {
	buf, err := m.Pack()
	if err != nil {
		return err
	}
	_, err = s.sendconn.WriteTo(buf, ipv4Addr)
	return err
}
