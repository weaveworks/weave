package nameserver

import (
	"testing"
	"github.com/miekg/dns"
	wt "github.com/zettio/weave/testing"
	"fmt"
	"net"
)

func TestCache(t *testing.T) {
	l, err := NewCache(128)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	for i := 0; i < 256; i++ {
		questionMsg := new(dns.Msg)
		questionMsg.SetQuestion(fmt.Sprintf("name%d", i), dns.TypeA)
		questionMsg.RecursionDesired = true

		question := &questionMsg.Question[0]

		ip := net.ParseIP(fmt.Sprintf("10.0.1.%d", i))
		ips := []net.IP{ip}
		reply := makeAddressReply(questionMsg, question, ips)

		l.Put(question, reply)
	}

	wt.AssertEqualInt(t, l.Len(), 128, "cache length")
}
