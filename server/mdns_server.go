package weavedns

import (
	"github.com/miekg/dns"
	"net"
)

type MDNSServer struct {
	sendconn *net.UDPConn
}

func NewMDNSServer() (*MDNSServer, error) {
	//log.Println("minimalServer sending:", buf)
	// This is a bit of a kludge - per the RFC we should send responses from 5353, but that doesn't seem to work
	sendconn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, err
	}
	retval := &MDNSServer{sendconn: sendconn}
	return retval, nil
}

func (s *MDNSServer) SendResponse(m *dns.Msg) error {
	buf, err := m.Pack()
	if err != nil {
		return err
	}
	_, err = s.sendconn.WriteTo(buf, ipv4Addr)
	return err
}
