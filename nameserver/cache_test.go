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

		l.Put(questionMsg, reply, 0, insTime)
	}

	wt.AssertEqualInt(t, l.Len(), cacheLen, "cache length")

	minExpectedTime := insTime.Add(time.Duration(cacheLen) * time.Second)
	t.Logf("Checking all remaining entries expire after insert_time + %d secs='%s'", cacheLen, minExpectedTime)
	for _, entry := range l.entries {
		if entry.validUntil.Before(minExpectedTime) {
			t.Fatalf("Entry valid until %s", entry.validUntil)
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
	resp, err := l.Get(questionMsg, minUDPSize, time.Now())
	wt.AssertNoErr(t, err)
	if resp != nil {
		t.Logf("Got '%s'", resp)
		t.Fatalf("ERROR: Did not expect a reponse from Get() yet")
	}
	t.Logf("Trying to get it again")
	resp, err = l.Get(questionMsg, minUDPSize, time.Now())
	wt.AssertNoErr(t, err)
	if resp != nil {
		t.Logf("Got '%s'", resp)
		t.Fatalf("ERROR: Did not expect a reponse from Get() yet")
	}

	t.Logf("Inserting the reply")
	reply1 := makeAddressReply(questionMsg, question, []net.IP{net.ParseIP("10.0.1.1")})
	l.Put(questionMsg, reply1, 0, time.Now())

	timeGet1 := time.Now()
	t.Logf("Checking we can Get() the reply now")
	resp, err = l.Get(questionMsg, minUDPSize, timeGet1)
	wt.AssertNoErr(t, err)
	wt.AssertTrue(t, resp != nil, "reponse from Get()")
	t.Logf("Received '%s'", resp.Answer[0])
	wt.AssertType(t, resp.Answer[0], (*dns.A)(nil), "DNS record")
	ttlGet1 := resp.Answer[0].Header().Ttl

	t.Logf("Checking a Wait() with timeout=0 gets the same result")
	resp, err = l.Wait(questionMsg, time.Duration(0)*time.Second, minUDPSize, time.Now())
	wt.AssertNoErr(t, err)
	wt.AssertTrue(t, resp != nil, "reponse from a Wait(timeout=0)")
	t.Logf("Received '%s'", resp.Answer[0])
	wt.AssertType(t, resp.Answer[0], (*dns.A)(nil), "DNS record")

	timeGet2 := timeGet1.Add(time.Duration(1) * time.Second)
	t.Logf("Checking that a second Get(), after 1 second, gets the same result, but with reduced TTL")
	resp, err = l.Get(questionMsg, minUDPSize, timeGet2)
	wt.AssertNoErr(t, err)
	wt.AssertTrue(t, resp != nil, "reponse from a second Get()")
	t.Logf("Received '%s'", resp.Answer[0])
	wt.AssertType(t, resp.Answer[0], (*dns.A)(nil), "DNS record")
	ttlGet2 := resp.Answer[0].Header().Ttl
	wt.AssertEqualInt(t, int(ttlGet1-ttlGet2), 1, "TTL difference")

	timeGet3 := timeGet1.Add(time.Duration(localTTL) * time.Second)
	t.Logf("Checking that a third Get(), after %d second, gets no result", localTTL)
	resp, err = l.Get(questionMsg, minUDPSize, timeGet3)
	wt.AssertNoErr(t, err)
	if resp != nil {
		t.Logf("Got '%s'", resp)
		t.Fatalf("ERROR: Did NOT expect a reponse from the second Get()")
	}

	t.Logf("Checking that an Remove() results in Get() returning nothing")
	replyTemp := makeAddressReply(questionMsg, question, []net.IP{net.ParseIP("10.0.9.9")})
	l.Put(questionMsg, replyTemp, 0, time.Now())
	lenBefore := l.Len()
	l.Remove(question)
	wt.AssertEqualInt(t, l.Len(), lenBefore-1, "cache length")
	l.Remove(question) // do it again: should have no effect...
	wt.AssertEqualInt(t, l.Len(), lenBefore-1, "cache length")

	resp, err = l.Get(questionMsg, minUDPSize, timeGet1)
	wt.AssertNoErr(t, err)
	wt.AssertTrue(t, resp == nil, "reponse from the Get() after a Remove()")

	t.Logf("Inserting a two replies for the same query")
	timePut2 := time.Now()
	reply2 := makeAddressReply(questionMsg, question, []net.IP{net.ParseIP("10.0.1.2")})
	l.Put(questionMsg, reply2, 0, timePut2)
	timePut3 := timePut2.Add(time.Duration(1) * time.Second)
	reply3 := makeAddressReply(questionMsg, question, []net.IP{net.ParseIP("10.0.1.3")})
	l.Put(questionMsg, reply3, 0, timePut3)

	t.Logf("Checking we get the last one...")
	resp, err = l.Get(questionMsg, minUDPSize, timePut3)
	wt.AssertNoErr(t, err)
	wt.AssertTrue(t, resp != nil, "reponse from the Get()")
	t.Logf("Received '%s'", resp.Answer[0])
	wt.AssertType(t, resp.Answer[0], (*dns.A)(nil), "DNS record")
	wt.AssertEqualString(t, resp.Answer[0].(*dns.A).A.String(), "10.0.1.3", "IP address")
	wt.AssertEqualInt(t, int(resp.Answer[0].Header().Ttl), int(localTTL), "TTL")

	resp, err = l.Get(questionMsg, minUDPSize, timePut3.Add(time.Duration(localTTL-1)*time.Second))
	wt.AssertNoErr(t, err)
	wt.AssertTrue(t, resp != nil, "reponse from the Get()")
	t.Logf("Received '%s'", resp.Answer[0])
	wt.AssertType(t, resp.Answer[0], (*dns.A)(nil), "DNS record")
	wt.AssertEqualString(t, resp.Answer[0].(*dns.A).A.String(), "10.0.1.3", "IP address")
	wt.AssertEqualInt(t, int(resp.Answer[0].Header().Ttl), 1, "TTL")

	t.Logf("Checking we get empty replies when they are expired...")
	lenBefore = l.Len()
	resp, err = l.Get(questionMsg, minUDPSize, timePut3.Add(time.Duration(localTTL)*time.Second))
	wt.AssertNoErr(t, err)
	if resp != nil {
		t.Logf("Received '%s'", resp.Answer[0])
		t.Fatalf("ERROR: Did NOT expect a reponse from the Get()")
	}
	wt.AssertEqualInt(t, l.Len(), lenBefore-1, "cache length (after getting an expired entry)")

	questionMsg2 := new(dns.Msg)
	questionMsg2.SetQuestion("some.other.name", dns.TypeA)
	questionMsg2.RecursionDesired = true
	question2 := &questionMsg2.Question[0]

	t.Logf("Trying to Get() a name")
	resp, err = l.Get(questionMsg2, minUDPSize, time.Now())
	wt.AssertNoErr(t, err)
	wt.AssertNil(t, resp, "reponse from Get() yet")

	t.Logf("Checking that an Remove() between Get() and Put() does not break things")
	replyTemp2 := makeAddressReply(questionMsg2, question2, []net.IP{net.ParseIP("10.0.9.9")})
	l.Remove(question2)
	l.Put(questionMsg2, replyTemp2, 0, time.Now())
	resp, err = l.Get(questionMsg2, minUDPSize, time.Now())
	wt.AssertNoErr(t, err)
	wt.AssertNotNil(t, resp, "reponse from Get()")
	resp, err = l.Wait(questionMsg2, time.Duration(0)*time.Second, minUDPSize, time.Now())
	wt.AssertNoErr(t, err)
	wt.AssertNotNil(t, resp, "reponse from Get()")

	questionMsg3 := new(dns.Msg)
	questionMsg3.SetQuestion("some.other.name", dns.TypeA)
	questionMsg3.RecursionDesired = true
	question3 := &questionMsg3.Question[0]

	t.Logf("Checking that a entry with CacheNoLocalReplies return an error")
	timePut3 = time.Now()
	l.Put(questionMsg3, nil, CacheNoLocalReplies, timePut3)
	resp, err = l.Get(questionMsg3, minUDPSize, timePut3)
	wt.AssertNil(t, resp, "Get() response with CacheNoLocalReplies")
	wt.AssertNotNil(t, err, "Get() error with CacheNoLocalReplies")

	timeExpiredGet3 := timePut3.Add(time.Second * time.Duration(negLocalTTL+1))
	t.Logf("Checking that we get an expired response after %f seconds", timeExpiredGet3.Sub(timePut3).Seconds())
	resp, err = l.Get(questionMsg3, minUDPSize, timeExpiredGet3)
	wt.AssertNil(t, resp, "expired Get() response with CacheNoLocalReplies")
	wt.AssertNil(t, err, "expired Get() error with CacheNoLocalReplies")

	l.Remove(question3)
	t.Logf("Checking that Put&Get with CacheNoLocalReplies with a Remove in the middle returns nothing")
	l.Put(questionMsg3, nil, CacheNoLocalReplies, time.Now())
	l.Remove(question3)
	resp, err = l.Get(questionMsg3, minUDPSize, time.Now())
	wt.AssertNil(t, resp, "Get() reponse with CacheNoLocalReplies")
	wt.AssertNil(t, err, "Get() error with CacheNoLocalReplies")
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
			_, err := l.Get(request, minUDPSize, time.Now())
			wt.AssertNoErr(t, err)
			t.Logf("Waiting for %s...", request.Question[0].Name)
			r, err := l.Wait(request, 1*time.Second, minUDPSize, time.Now())
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
		l.Put(requestMsg, reply, 0, time.Now())
	}
}
