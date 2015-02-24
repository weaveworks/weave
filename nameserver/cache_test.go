package nameserver

import (
	"fmt"
	"github.com/miekg/dns"
	. "github.com/zettio/weave/common"
	wt "github.com/zettio/weave/testing"
	"net"
	"testing"
	"time"
)

// Check that the cache keeps its intended capacity constant
func TestCacheLength(t *testing.T) {
	InitDefaultLogging(true)

	const cacheLen = 128

	l, err := NewCache(cacheLen)
	wt.AssertNoErr(t, err)

	for i := 0; i < cacheLen * 2; i++ {
		questionMsg := new(dns.Msg)
		questionMsg.SetQuestion(fmt.Sprintf("name%d", i), dns.TypeA)
		questionMsg.RecursionDesired = true

		question := &questionMsg.Question[0]

		ip := net.ParseIP(fmt.Sprintf("10.0.1.%d", i))
		ips := []net.IP{ip}
		reply := makeAddressReply(questionMsg, question, ips)

		l.Put(questionMsg, reply, 0, time.Now())
	}

	wt.AssertEqualInt(t, l.Len(), cacheLen, "cache length")
}


// Check that waiters are unblocked when the name they are waiting for is inserted
func TestCacheBlockingOps(t *testing.T) {
	InitDefaultLogging(true)

	const cacheLen = 256

	l, err := NewCache(cacheLen)
	wt.AssertNoErr(t, err)

	requests := []*dns.Msg{}

	t.Logf("Starting 256 queries that will block")
	for i := 0; i < cacheLen; i++ {
		questionName := fmt.Sprintf("name%d", i)
		questionMsg := new(dns.Msg)
		questionMsg.SetQuestion(questionName, dns.TypeA)
		questionMsg.RecursionDesired = true

		requests = append(requests, questionMsg)

		go func(request *dns.Msg) {
			t.Logf("Querying about %s...", request.Question[0].Name)
			_, err := l.Get(request, time.Now())
			wt.AssertNoErr(t, err)
			t.Logf("Waiting for %s...", request.Question[0].Name)
			r, err := l.Wait(request, 1 * time.Second, time.Now())
			t.Logf("Obtained response for %s:\n%s", request.Question[0].Name, r)
			wt.AssertNoErr(t, err)
		}(questionMsg)
	}

	// insert the IPs for those names
	for i, requestMsg := range requests {
		ip := net.ParseIP(fmt.Sprintf("10.0.1.%d", i))
		ips := []net.IP{ip}
		reply := makeAddressReply(requestMsg, &requestMsg.Question[0], ips)
		l.Put(requestMsg, reply, 0, time.Now())
	}
}


