package nameserver

import (
	"net"
	"testing"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
	. "github.com/weaveworks/weave/common"
)

// Check that we can prune an answer
func TestPrune(t *testing.T) {
	EnableDebugLogging(testing.Verbose())
	Log.Infoln("TestPrune starting")

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

	reply := makeAddressReply(questionMsg, question, records, DefaultLocalTTL)
	reply.Answer[0].Header().Ttl = DefaultLocalTTL

	pruned := pruneAnswers(reply.Answer, 1)
	require.Equal(t, 1, len(pruned), "wrong number of answers")

	pruned = pruneAnswers(reply.Answer, 2)
	require.Equal(t, 2, len(pruned), "wrong number of answers")

	pruned = pruneAnswers(reply.Answer, 0)
	require.Equal(t, len(records), len(pruned), "wrong number of answers")
}
