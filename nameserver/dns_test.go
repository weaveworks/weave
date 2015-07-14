package nameserver

import (
	"fmt"
	"math/rand"
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"

	"github.com/weaveworks/weave/net/address"
	"github.com/weaveworks/weave/router"
)

func TestTruncation(t *testing.T) {
	//common.SetLogLevel("debug")

	peername, err := router.PeerNameFromString("00:00:00:02:00:00")
	require.Nil(t, err)
	nameserver := New(peername, nil, nil, "")
	dnsserver, err := NewDNSServer(nameserver, "weave.local.", 0, 30, 5*time.Second)
	require.Nil(t, err)
	udpPort := dnsserver.servers[0].PacketConn.LocalAddr().(*net.UDPAddr).Port
	tcpPort := dnsserver.servers[1].Listener.Addr().(*net.TCPAddr).Port
	go dnsserver.ActivateAndServe()
	defer dnsserver.Stop()

	// Add 100 mappings to nameserver
	addrs := []address.Address{}
	for i := address.Address(0); i < 100; i++ {
		addrs = append(addrs, i)
		nameserver.AddEntry("foo.weave.local.", "", peername, i)
	}

	doRequest := func(client *dns.Client, request *dns.Msg, port int) *dns.Msg {
		request.SetQuestion("foo.weave.local.", dns.TypeA)
		response, _, err := client.Exchange(request, fmt.Sprintf("127.0.0.1:%d", port))
		require.Nil(t, err)
		return response
	}

	// do a udp query, ensure we get a truncated response
	{
		udpClient := dns.Client{Net: "udp", UDPSize: minUDPSize}
		response := doRequest(&udpClient, &dns.Msg{}, udpPort)
		require.Nil(t, err)
		require.True(t, response.MsgHdr.Truncated)
		require.True(t, len(response.Answer) < 100)
	}

	// do a udp query with big size, ensure we don't get a truncated response
	{
		udpClient := dns.Client{Net: "udp", UDPSize: 65535}
		request := &dns.Msg{}
		request.SetEdns0(65535, false)
		response := doRequest(&udpClient, request, udpPort)
		require.False(t, response.MsgHdr.Truncated)
		require.Equal(t, len(response.Answer), 100)
	}

	// do a tcp query, ensure we don't get a truncated response
	{
		tcpClient := dns.Client{Net: "tcp"}
		response := doRequest(&tcpClient, &dns.Msg{}, tcpPort)
		require.False(t, response.MsgHdr.Truncated)
		require.Equal(t, len(response.Answer), 100)
	}
}

func TestTruncateResponse(t *testing.T) {

	header := dns.RR_Header{
		Name:   "host.domain.com",
		Rrtype: dns.TypePTR,
		Class:  dns.ClassINET,
		Ttl:    30,
	}

	for i := 0; i < 10000; i++ {
		// generate a random msg
		response := &dns.Msg{}
		numAnswers := 40 + rand.Intn(200)
		response.Answer = make([]dns.RR, numAnswers)
		for j := 0; j < numAnswers; j++ {
			response.Answer[j] = &dns.A{Hdr: header, A: address.Address(j).IP4()}
		}

		// pick a random max size, truncate response to that, check it
		maxSize := 512 + rand.Intn(response.Len()-512)
		truncateResponse(response, maxSize)
		require.True(t, response.Len() <= maxSize)
	}
}
