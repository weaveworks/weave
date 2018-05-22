package npc

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/types"

	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/npc/iptables"
)

type ruleHost interface {
	// getRuleSpec returns source or destination specification and comment which
	// are used in an iptables rule.
	//
	// If src=true then the rulespec is for source, otherwise - for destination.
	getRuleSpec(src bool) ([]string, string)
}

type ruleSpec struct {
	key        string
	args       []string
	policyType policyType
}

func newRuleSpec(policyType policyType, proto *string, srcHost, dstHost ruleHost, dstPort *string) *ruleSpec {
	args := []string{}
	if proto != nil {
		args = append(args, "-p", *proto)
	}
	srcComment := "anywhere"
	if srcHost != nil {
		rule, comment := srcHost.getRuleSpec(true)
		args = append(args, rule...)
		srcComment = comment
	}
	dstComment := "anywhere"
	if dstHost != nil {
		rule, comment := dstHost.getRuleSpec(false)
		args = append(args, rule...)
		dstComment = comment
	}
	if dstPort != nil {
		args = append(args, "--dport", *dstPort)
	}
	args = append(args, "-j", "ACCEPT")
	// NOTE: if you remove the comment bellow, then embed `policyType` into `key`.
	// Otherwise, the rule won't be provisioned if it exists for other policy type.
	args = append(args, "-m", "comment", "--comment", fmt.Sprintf("%s -> %s (%s)", srcComment, dstComment, policyTypeStr(policyType)))
	key := strings.Join(args, " ")

	return &ruleSpec{key, args, policyType}
}

type ruleSet struct {
	ipt   iptables.Interface
	users map[string]map[types.UID]struct{}
}

func newRuleSet(ipt iptables.Interface) *ruleSet {
	return &ruleSet{ipt, make(map[string]map[types.UID]struct{})}
}

func (rs *ruleSet) deprovision(user types.UID, current, desired map[string]*ruleSpec) error {
	for key, spec := range current {
		if _, found := desired[key]; !found {
			delete(rs.users[key], user)
			if len(rs.users[key]) == 0 {
				chain := rulesChainByPolicyType(spec.policyType)
				common.Log.Infof("deleting rule %v from %q chain", spec.args, chain)
				if err := rs.ipt.Delete(TableFilter, chain, spec.args...); err != nil {
					return err
				}
				delete(rs.users, key)
			}
		}
	}

	return nil
}

func (rs *ruleSet) provision(user types.UID, current, desired map[string]*ruleSpec) error {
	for key, spec := range desired {
		if _, found := current[key]; !found {
			if _, found := rs.users[key]; !found {
				chain := rulesChainByPolicyType(spec.policyType)
				common.Log.Infof("adding rule %v to %q chain", spec.args, chain)
				if err := rs.ipt.Append(TableFilter, chain, spec.args...); err != nil {
					return err
				}
				rs.users[key] = make(map[types.UID]struct{})
			}
			rs.users[key][user] = struct{}{}
		}
	}

	return nil
}

func rulesChainByPolicyType(policyType policyType) string {
	if policyType == egressPolicy {
		return EgressCustomChain
	}
	return IngressChain
}
