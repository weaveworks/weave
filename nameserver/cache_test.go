package nameserver

import (
	"fmt"
	"github.com/benbjohnson/clock"
	"github.com/miekg/dns"
	. "github.com/weaveworks/weave/common"
	wt "github.com/weaveworks/weave/testing"
	"net"
	"testing"
	"time"
)

// Check that the cache keeps its intended capacity constant
func TestCacheLength(t *testing.T) {
	InitDefaultLogging(testing.Verbose())
	Info.Println("TestCacheLength starting")

	const cacheLen = 128

	l, err := NewCache(cacheLen, nil)
	wt.AssertNoErr(t, err)

	insTime := time.Now()

	t.Logf("Inserting 256 questions in the cache at '%s', with TTL from 0 to 255", insTime)
	for i := 0; i < cacheLen*2; i++ {
		name := fmt.Sprintf("name%d", i)
		questionMsg := new(dns.Msg)
		questionMsg.SetQuestion(name, dns.TypeA)
		questionMsg.RecursionDesired = true

		question := &questionMsg.Question[0]

		ip := net.ParseIP(fmt.Sprintf("10.0.1.%d", i))
		records := []ZoneRecord{Record{name, ip, 0, 0, 0}}

		reply := makeAddressReply(questionMsg, question, records)
		reply.Answer[0].Header().Ttl = uint32(i)

		l.Put(questionMsg, reply, 0, 0)
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
	InitDefaultLogging(testing.Verbose())
	Info.Println("TestCacheEntries starting")

	Info.Println("Checking cache consistency")

	const cacheLen = 128
	clk := clock.NewMock()

	l, err := NewCache(cacheLen, clk)
	wt.AssertNoErr(t, err)

	questionMsg := new(dns.Msg)
	questionMsg.SetQuestion("some.name", dns.TypeA)
	questionMsg.RecursionDesired = true

	question := &questionMsg.Question[0]

	t.Logf("Trying to get a name")
	resp, err := l.Get(questionMsg, minUDPSize)
	wt.AssertNoErr(t, err)
	if resp != nil {
		t.Logf("Got\n%s", resp)
		t.Fatalf("ERROR: Did not expect a reponse from Get() yet")
	}
	t.Logf("Trying to get it again")
	resp, err = l.Get(questionMsg, minUDPSize)
	wt.AssertNoErr(t, err)
	if resp != nil {
		t.Logf("Got\n%s", resp)
		t.Fatalf("ERROR: Did not expect a reponse from Get() yet")
	}

	t.Logf("Inserting the reply")
	records := []ZoneRecord{Record{"some.name", net.ParseIP("10.0.1.1"), 0, 0, 0}}
	reply1 := makeAddressReply(questionMsg, question, records)
	l.Put(questionMsg, reply1, nullTTL, 0)

	t.Logf("Checking we can Get() the reply now")
	resp, err = l.Get(questionMsg, minUDPSize)
	wt.AssertNoErr(t, err)
	wt.AssertTrue(t, resp != nil, "reponse from Get()")
	t.Logf("Received '%s'", resp.Answer[0])
	wt.AssertType(t, resp.Answer[0], (*dns.A)(nil), "DNS record")
	ttlGet1 := resp.Answer[0].Header().Ttl

	clk.Add(time.Duration(1) * time.Second)
	t.Logf("Checking that a second Get(), after 1 second, gets the same result, but with reduced TTL")
	resp, err = l.Get(questionMsg, minUDPSize)
	wt.AssertNoErr(t, err)
	wt.AssertTrue(t, resp != nil, "reponse from a second Get()")
	t.Logf("Received '%s'", resp.Answer[0])
	wt.AssertType(t, resp.Answer[0], (*dns.A)(nil), "DNS record")
	ttlGet2 := resp.Answer[0].Header().Ttl
	wt.AssertEqualInt(t, int(ttlGet1-ttlGet2), 1, "TTL difference")

	clk.Add(time.Duration(localTTL) * time.Second)
	t.Logf("Checking that a third Get(), after %d second, gets no result", localTTL)
	resp, err = l.Get(questionMsg, minUDPSize)
	wt.AssertNoErr(t, err)
	if resp != nil {
		t.Logf("Got\n%s", resp)
		t.Fatalf("ERROR: Did NOT expect a reponse from the second Get()")
	}

	t.Logf("Checking that an Remove() results in Get() returning nothing")
	records = []ZoneRecord{Record{"some.name", net.ParseIP("10.0.9.9"), 0, 0, 0}}
	replyTemp := makeAddressReply(questionMsg, question, records)
	l.Put(questionMsg, replyTemp, nullTTL, 0)
	lenBefore := l.Len()
	l.Remove(question)
	wt.AssertEqualInt(t, l.Len(), lenBefore-1, "cache length")
	l.Remove(question) // do it again: should have no effect...
	wt.AssertEqualInt(t, l.Len(), lenBefore-1, "cache length")

	resp, err = l.Get(questionMsg, minUDPSize)
	wt.AssertNoErr(t, err)
	wt.AssertTrue(t, resp == nil, "reponse from the Get() after a Remove()")

	t.Logf("Inserting a two replies for the same query")
	records = []ZoneRecord{Record{"some.name", net.ParseIP("10.0.1.2"), 0, 0, 0}}
	reply2 := makeAddressReply(questionMsg, question, records)
	l.Put(questionMsg, reply2, nullTTL, 0)
	clk.Add(time.Duration(1) * time.Second)
	records = []ZoneRecord{Record{"some.name", net.ParseIP("10.0.1.3"), 0, 0, 0}}
	reply3 := makeAddressReply(questionMsg, question, records)
	l.Put(questionMsg, reply3, nullTTL, 0)

	t.Logf("Checking we get the last one...")
	resp, err = l.Get(questionMsg, minUDPSize)
	wt.AssertNoErr(t, err)
	wt.AssertTrue(t, resp != nil, "reponse from the Get()")
	t.Logf("Received '%s'", resp.Answer[0])
	wt.AssertType(t, resp.Answer[0], (*dns.A)(nil), "DNS record")
	wt.AssertEqualString(t, resp.Answer[0].(*dns.A).A.String(), "10.0.1.3", "IP address")
	wt.AssertEqualInt(t, int(resp.Answer[0].Header().Ttl), int(localTTL), "TTL")

	clk.Add(time.Duration(localTTL-1) * time.Second)
	resp, err = l.Get(questionMsg, minUDPSize)
	wt.AssertNoErr(t, err)
	wt.AssertTrue(t, resp != nil, "reponse from the Get()")
	t.Logf("Received '%s'", resp.Answer[0])
	wt.AssertType(t, resp.Answer[0], (*dns.A)(nil), "DNS record")
	wt.AssertEqualString(t, resp.Answer[0].(*dns.A).A.String(), "10.0.1.3", "IP address")
	wt.AssertEqualInt(t, int(resp.Answer[0].Header().Ttl), 1, "TTL")

	t.Logf("Checking we get empty replies when they are expired...")
	lenBefore = l.Len()
	clk.Add(time.Duration(localTTL) * time.Second)
	resp, err = l.Get(questionMsg, minUDPSize)
	wt.AssertNoErr(t, err)
	if resp != nil {
		t.Logf("Got\n%s", resp.Answer[0])
		t.Fatalf("ERROR: Did NOT expect a reponse from the Get()")
	}
	wt.AssertEqualInt(t, l.Len(), lenBefore-1, "cache length (after getting an expired entry)")

	questionMsg2 := new(dns.Msg)
	questionMsg2.SetQuestion("some.other.name", dns.TypeA)
	questionMsg2.RecursionDesired = true
	question2 := &questionMsg2.Question[0]

	t.Logf("Trying to Get() a name")
	resp, err = l.Get(questionMsg2, minUDPSize)
	wt.AssertNoErr(t, err)
	wt.AssertNil(t, resp, "reponse from Get() yet")

	t.Logf("Checking that an Remove() between Get() and Put() does not break things")
	records = []ZoneRecord{Record{"some.name", net.ParseIP("10.0.9.9"), 0, 0, 0}}
	replyTemp2 := makeAddressReply(questionMsg2, question2, records)
	l.Remove(question2)
	l.Put(questionMsg2, replyTemp2, nullTTL, 0)
	resp, err = l.Get(questionMsg2, minUDPSize)
	wt.AssertNoErr(t, err)
	wt.AssertNotNil(t, resp, "reponse from Get()")

	questionMsg3 := new(dns.Msg)
	questionMsg3.SetQuestion("some.other.name", dns.TypeA)
	questionMsg3.RecursionDesired = true
	question3 := &questionMsg3.Question[0]

	t.Logf("Checking that a entry with CacheNoLocalReplies return an error")
	l.Put(questionMsg3, nil, nullTTL, CacheNoLocalReplies)
	resp, err = l.Get(questionMsg3, minUDPSize)
	wt.AssertNil(t, resp, "Get() response with CacheNoLocalReplies")
	wt.AssertNotNil(t, err, "Get() error with CacheNoLocalReplies")

	clk.Add(time.Second * time.Duration(negLocalTTL+1))
	t.Logf("Checking that we get an expired response after %f seconds", negLocalTTL)
	resp, err = l.Get(questionMsg3, minUDPSize)
	wt.AssertNil(t, resp, "expired Get() response with CacheNoLocalReplies")
	wt.AssertNil(t, err, "expired Get() error with CacheNoLocalReplies")

	l.Remove(question3)
	t.Logf("Checking that Put&Get with CacheNoLocalReplies with a Remove in the middle returns nothing")
	l.Put(questionMsg3, nil, nullTTL, CacheNoLocalReplies)
	l.Remove(question3)
	resp, err = l.Get(questionMsg3, minUDPSize)
	wt.AssertNil(t, resp, "Get() reponse with CacheNoLocalReplies")
	wt.AssertNil(t, err, "Get() error with CacheNoLocalReplies")
}
