package nameserver

import (
	"github.com/miekg/dns"
	. "github.com/weaveworks/weave/common"
	wt "github.com/weaveworks/weave/testing"
	"net"
	"testing"
)

// Check that we can prune an answer
func TestPrune(t *testing.T) {
	InitDefaultLogging(testing.Verbose())
	Info.Println("TestPrune starting")

	questionMsg := new(dns.Msg)
	questionMsg.SetQuestion("name", dns.TypeA)
	questionMsg.RecursionDesired = true
	question := &questionMsg.Question[0]
	records := []ZoneRecord{
		Record{"name", net.ParseIP("10.0.1.1"), 0, 0, 0},
		Record{"name", net.ParseIP("10.0.1.2"), 0, 0, 0},
		Record{"name", net.ParseIP("10.0.1.3"), 0, 0, 0},
		Record{"name", net.ParseIP("10.0.1.4"), 0, 0, 0},
	}

	reply := makeAddressReply(questionMsg, question, records)
	reply.Answer[0].Header().Ttl = localTTL

	pruned := pruneAnswers(reply.Answer, 1)
	wt.AssertEqualInt(t, len(pruned), 1, "wrong number of answers")

	pruned = pruneAnswers(reply.Answer, 2)
	wt.AssertEqualInt(t, len(pruned), 2, "wrong number of answers")

	pruned = pruneAnswers(reply.Answer, 0)
	wt.AssertEqualInt(t, len(pruned), len(records), "wrong number of answers")
}
