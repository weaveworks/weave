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

	insTime := time.Now()

	t.Logf("Inserting 256 questions in the cache at '%s', with TTL from 0 to 255", insTime)
	for i := 0; i < cacheLen*2; i++ {
		questionMsg := new(dns.Msg)
		questionMsg.SetQuestion(fmt.Sprintf("name%d", i), dns.TypeA)
		questionMsg.RecursionDesired = true

		question := &questionMsg.Question[0]

		ip := net.ParseIP(fmt.Sprintf("10.0.1.%d", i))
		ips := []net.IP{ip}
		reply := makeAddressReply(questionMsg, question, ips)
		reply.Answer[0].Header().Ttl = uint32(i)

		l.Put(questionMsg, reply, protUdp, 0, insTime)
	}

	wt.AssertEqualInt(t, l.Len(), cacheLen, "cache length")

	minExpectedTime := insTime.Add(time.Duration(cacheLen) * time.Second)
	t.Logf("Checking all remaining entries expire after insert_time + %d secs='%s'", cacheLen, minExpectedTime)
	for _, entry := range l.entries {
		if entry.validUntil.Before(minExpectedTime) {
			t.Errorf("Entry valid until %s", entry.validUntil)
		}
	}
}

// Check that the cache entries are ok
func TestCacheEntries(t *testing.T) {
	InitDefaultLogging(true)

	Info.Println("Checking cache consistency")

	const cacheLen = 128

	l, err := NewCache(cacheLen)
	wt.AssertNoErr(t, err)

	questionMsg := new(dns.Msg)
	questionMsg.SetQuestion("some.name", dns.TypeA)
	questionMsg.RecursionDesired = true

	question := &questionMsg.Question[0]

	t.Logf("Trying to get a name")
	resp, err := l.Get(questionMsg, protUdp, time.Now())
	wt.AssertNoErr(t, err)
	if resp != nil {
		t.Logf("Got '%s'", resp)
		t.Error("ERROR: Did not expect a reponse from Get() yet")
	}
	t.Logf("Trying to get it again")
	resp, err = l.Get(questionMsg, protUdp, time.Now())
	wt.AssertNoErr(t, err)
	if resp != nil {
		t.Logf("Got '%s'", resp)
		t.Error("ERROR: Did not expect a reponse from Get() yet")
	}

	t.Logf("Inserting the reply")
	ip := net.ParseIP("10.0.1.1")
	ips := []net.IP{ip}
	reply1 := makeAddressReply(questionMsg, question, ips)
	l.Put(questionMsg, reply1, protUdp, 0, time.Now())

	timeGet1 := time.Now()
	t.Logf("Checking we can Get() the reply now")
	resp, err = l.Get(questionMsg, protUdp, timeGet1)
	wt.AssertNoErr(t, err)
	if resp == nil {
		t.Error("ERROR: Did expect a reponse from Get()")
	}
	t.Logf("Received '%s'", resp.Answer[0])
	wt.AssertType(t, resp.Answer[0], (*dns.A)(nil), "DNS record")
	ttlGet1 := resp.Answer[0].Header().Ttl

	t.Logf("Checking that a Get() for different protocol return nothing")
	resp, err = l.Get(questionMsg, protTcp, timeGet1)
	wt.AssertNoErr(t, err)
	if resp != nil {
		t.Logf("Received '%s'", resp.Answer[0])
		t.Error("ERROR: Did NOT expect a reponse from Get() with TCP")
	}

	t.Logf("Checking a Wait() with timeout=0 gets the same result")
	resp, err = l.Wait(questionMsg, protUdp, time.Duration(0)*time.Second, time.Now())
	wt.AssertNoErr(t, err)
	if resp == nil {
		t.Error("ERROR: Did expect a reponse from Wait(timeout=0)")
	}
	t.Logf("Received '%s'", resp.Answer[0])
	wt.AssertType(t, resp.Answer[0], (*dns.A)(nil), "DNS record")

	timeGet2 := timeGet1.Add(time.Duration(1) * time.Second)
	t.Logf("Checking that a second Get(), after 1 second, gets the same result, but with reduced TTL")
	resp, err = l.Get(questionMsg, protUdp, timeGet2)
	wt.AssertNoErr(t, err)
	if resp == nil {
		t.Error("ERROR: Did expect a reponse from the second Get()")
	}
	t.Logf("Received '%s'", resp.Answer[0])
	wt.AssertType(t, resp.Answer[0], (*dns.A)(nil), "DNS record")
	ttlGet2 := resp.Answer[0].Header().Ttl
	if ttlGet1-ttlGet2 != 1 {
		t.Errorf("ERROR: TTL difference is not 1 (it is %d)", ttlGet1-ttlGet2)
	}

	timeGet3 := timeGet1.Add(time.Duration(localTTL) * time.Second)
	t.Logf("Checking that a third Get(), after %d second, gets no result", localTTL)
	resp, err = l.Get(questionMsg, protUdp, timeGet3)
	wt.AssertNoErr(t, err)
	if resp != nil {
		t.Logf("Got '%s'", resp)
		t.Error("ERROR: Did NOT expect a reponse from the second Get()")
	}

	t.Logf("Checking that an Remove() results in Get() returning nothing")
	l.Remove(question, protUdp)
	l.Remove(question, protUdp) // do it again: should have no effect...
	resp, err = l.Get(questionMsg, protUdp, timeGet1)
	wt.AssertNoErr(t, err)
	if resp != nil {
		t.Logf("Got '%s'", resp)
		t.Error("ERROR: Did NOT expect a reponse after an Invalidate()")
	}

	t.Logf("Inserting a two UDP and one TCP replies for the same query")
	timePut2 := time.Now()
	reply2 := makeAddressReply(questionMsg, question, []net.IP{net.ParseIP("10.0.1.2")})
	l.Put(questionMsg, reply2, protUdp, 0, timePut2)
	timePut3 := timePut2.Add(time.Duration(1) * time.Second)
	reply3 := makeAddressReply(questionMsg, question, []net.IP{net.ParseIP("10.0.1.3")})
	l.Put(questionMsg, reply3, protUdp, 0, timePut3)
	timePut4 := timePut3
	reply4 := makeAddressReply(questionMsg, question, []net.IP{net.ParseIP("10.0.10.10")})
	l.Put(questionMsg, reply4, protTcp, 0, timePut4)

	t.Logf("Checking we get the last one...")
	resp, err = l.Get(questionMsg, protUdp, timePut3)
	wt.AssertNoErr(t, err)
	if resp == nil {
		t.Error("ERROR: Did expect a reponse from the Get()")
	}
	t.Logf("Received '%s'", resp.Answer[0])
	wt.AssertType(t, resp.Answer[0], (*dns.A)(nil), "DNS record")
	wt.AssertEqualString(t, resp.Answer[0].(*dns.A).A.String(), "10.0.1.3", "IP address")
	if resp.Answer[0].Header().Ttl != localTTL {
		t.Errorf("ERROR: TTL is not %d (it is %d)", localTTL, resp.Answer[0].Header().Ttl)
	}

	resp, err = l.Get(questionMsg, protTcp, timePut3.Add(time.Duration(localTTL - 1) * time.Second))
	wt.AssertNoErr(t, err)
	if resp == nil {
		t.Error("ERROR: Did expect a reponse from the Get()")
	}
	t.Logf("Received '%s'", resp.Answer[0])
	wt.AssertType(t, resp.Answer[0], (*dns.A)(nil), "DNS record")
	wt.AssertEqualString(t, resp.Answer[0].(*dns.A).A.String(), "10.0.10.10", "IP address")
	if resp.Answer[0].Header().Ttl != 1 {
		t.Errorf("ERROR: TTL is not 1 (it is %d)", resp.Answer[0].Header().Ttl)
	}

	t.Logf("Checking we get empty replies when they are expired...")
	resp, err = l.Get(questionMsg, protTcp, timePut3.Add(time.Duration(localTTL) * time.Second))
	wt.AssertNoErr(t, err)
	if resp != nil {
		t.Logf("Received '%s'", resp.Answer[0])
		t.Error("ERROR: Did NOT expect a reponse from the Get()")
	}
}

// Check that waiters are unblocked when the name they are waiting for is inserted
func TestCacheBlockingOps(t *testing.T) {
	InitDefaultLogging(true)

	const cacheLen = 256

	l, err := NewCache(cacheLen)
	wt.AssertNoErr(t, err)

	requests := []*dns.Msg{}

	t.Logf("Starting 256 queries that will block...")
	for i := 0; i < cacheLen; i++ {
		questionName := fmt.Sprintf("name%d", i)
		questionMsg := new(dns.Msg)
		questionMsg.SetQuestion(questionName, dns.TypeA)
		questionMsg.RecursionDesired = true

		requests = append(requests, questionMsg)

		go func(request *dns.Msg) {
			t.Logf("Querying about %s...", request.Question[0].Name)
			_, err := l.Get(request, protUdp, time.Now())
			wt.AssertNoErr(t, err)
			t.Logf("Waiting for %s...", request.Question[0].Name)
			r, err := l.Wait(request, protUdp, 1*time.Second, time.Now())
			t.Logf("Obtained response for %s:\n%s", request.Question[0].Name, r)
			wt.AssertNoErr(t, err)
		}(questionMsg)
	}

	// insert the IPs for those names
	for i, requestMsg := range requests {
		ip := net.ParseIP(fmt.Sprintf("10.0.1.%d", i))
		ips := []net.IP{ip}
		reply := makeAddressReply(requestMsg, &requestMsg.Question[0], ips)

		t.Logf("Inserting response for %s...", requestMsg.Question[0].Name)
		l.Put(requestMsg, reply, protUdp, 0, time.Now())
	}
}
