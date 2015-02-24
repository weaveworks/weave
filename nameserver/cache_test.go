package nameserver

import (
	"fmt"
	"github.com/miekg/dns"
	"github.com/zettio/weave/common"
	wt "github.com/zettio/weave/testing"
	"net"
	"testing"
	"time"
)

// Some simple tests for the cache
func TestCacheSimple(t *testing.T) {
	common.InitDefaultLogging(true)

	l, err := NewCache(128)
	wt.AssertNoErr(t, err)

	for i := 0; i < 256; i++ {
		questionMsg := new(dns.Msg)
		questionMsg.SetQuestion(fmt.Sprintf("name%d", i), dns.TypeA)
		questionMsg.RecursionDesired = true

		question := &questionMsg.Question[0]

		ip := net.ParseIP(fmt.Sprintf("10.0.1.%d", i))
		ips := []net.IP{ip}
		reply := makeAddressReply(questionMsg, question, ips)

		l.Put(questionMsg, reply, 0)
	}

	wt.AssertEqualInt(t, l.Len(), 128, "cache length")
}


// Some simple tests for the cache
func TestCacheBlockingOps(t *testing.T) {
	common.InitDefaultLogging(true)

	l, err := NewCache(256)
	wt.AssertNoErr(t, err)

	requests := []*dns.Msg{}

	// start 256 queries that will block
	for i := 0; i < 256; i++ {
		questionName := fmt.Sprintf("name%d", i)
		questionMsg := new(dns.Msg)
		questionMsg.SetQuestion(questionName, dns.TypeA)
		questionMsg.RecursionDesired = true

		requests = append(requests, questionMsg)

		go func(request *dns.Msg) {
			_, err := l.Get(request)
			wt.AssertNoErr(t, err)
			_, err = l.Wait(request, 1 * time.Second)
			wt.AssertNoErr(t, err)
		}(questionMsg)
	}

	// insert the IPs for those names
	for i, requestMsg := range requests {
		ip := net.ParseIP(fmt.Sprintf("10.0.1.%d", i))
		ips := []net.IP{ip}
		reply := makeAddressReply(requestMsg, &requestMsg.Question[0], ips)
		l.Put(requestMsg, reply, 0)
	}
}


