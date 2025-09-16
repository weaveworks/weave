package npc

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/rajch/weave/common"
	"github.com/rajch/weave/common/chains"
	"github.com/rajch/weave/net/ipset"
	"github.com/rajch/weave/npc/iptables"
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
	if srcHost != nil && !reflect.ValueOf(srcHost).IsNil() {
		rule, comment := srcHost.getRuleSpec(true)
		args = append(args, rule...)
		srcComment = comment
	}
	dstComment := "anywhere"
	if dstHost != nil && !reflect.ValueOf(dstHost).IsNil() {
		rule, comment := dstHost.getRuleSpec(false)
		args = append(args, rule...)
		dstComment = comment
	}
	if dstPort != nil {
		args = append(args, "--dport", *dstPort)
	}
	// NOTE: if you remove the comment bellow, then embed `policyType` into `key`.
	// Otherwise, the rule won't be provisioned if it exists for other policy type.
	args = append(args, "-m", "comment", "--comment", fmt.Sprintf("%s -> %s (%s)", srcComment, dstComment, policyTypeStr(policyType)))
	key := strings.Join(args, " ")

	return &ruleSpec{key, args, policyType}
}

func (spec *ruleSpec) iptChain() string {
	if spec.policyType == policyTypeEgress {
		return chains.EgressCustomChain
	}
	return chains.IngressChain
}

func (spec *ruleSpec) iptRuleSpecs() [][]string {
	if spec.policyType == policyTypeIngress {
		rule := make([]string, len(spec.args))
		copy(rule, spec.args)
		rule = append(rule, "-j", "ACCEPT")
		return [][]string{rule}
	}

	// policyTypeEgress
	ruleMark := make([]string, len(spec.args))
	copy(ruleMark, spec.args)
	ruleMark = append(ruleMark, "-j", chains.EgressMarkChain)
	ruleReturn := make([]string, len(spec.args))
	copy(ruleReturn, spec.args)
	ruleReturn = append(ruleReturn, "-j", "RETURN")
	return [][]string{ruleMark, ruleReturn}
}

type ruleSet struct {
	ipt   iptables.Interface
	users map[string]map[ipset.UID]struct{}
}

func newRuleSet(ipt iptables.Interface) *ruleSet {
	return &ruleSet{ipt, make(map[string]map[ipset.UID]struct{})}
}

func (rs *ruleSet) deprovision(user ipset.UID, current, desired map[string]*ruleSpec) error {
	for key, spec := range current {
		if _, found := desired[key]; !found {
			delete(rs.users[key], user)
			if len(rs.users[key]) == 0 {
				chain := spec.iptChain()
				for _, rule := range spec.iptRuleSpecs() {
					common.Log.Infof("deleting rule %v from %q chain", rule, chain)
					if err := rs.ipt.Delete(TableFilter, chain, rule...); err != nil {
						return err
					}
				}

				delete(rs.users, key)
			}
		}
	}

	return nil
}

func (rs *ruleSet) provision(user ipset.UID, current, desired map[string]*ruleSpec) error {
	for key, spec := range desired {
		if _, found := current[key]; !found {
			if _, found := rs.users[key]; !found {
				chain := spec.iptChain()
				for _, rule := range spec.iptRuleSpecs() {
					common.Log.Infof("adding rule %v to %q chain", rule, chain)
					if err := rs.ipt.Append(TableFilter, chain, rule...); err != nil {
						return err
					}
				}
				rs.users[key] = make(map[ipset.UID]struct{})
			}
			rs.users[key][user] = struct{}{}
		}
	}

	return nil
}
