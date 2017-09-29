package npc

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/types"

	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/npc/iptables"
)

type ruleSpec struct {
	key  string
	args []string
	// In the case of networkingv1 (non-legacy) netpol, we need to create a rule
	// that prevents traffic to dst from src which does not match any ingress
	// rule; allow=false marks such rules.
	allow bool
}

func newRuleSpecAllow(proto *string, srcHost *selectorSpec, dstHost *selectorSpec, dstPort *string) *ruleSpec {
	args := []string{}
	if proto != nil {
		args = append(args, "-p", *proto)
	}
	srcComment := "anywhere"
	if srcHost != nil {
		args = append(args, "-m", "set", "--match-set", string(srcHost.ipsetName), "src")
		if srcHost.nsName != "" {
			srcComment = fmt.Sprintf("pods: namespace: %s, selector: %s", srcHost.nsName, srcHost.key)
		} else {
			srcComment = fmt.Sprintf("namespaces: selector: %s", srcHost.key)
		}
	}
	dstComment := "anywhere"
	if dstHost != nil {
		args = append(args, "-m", "set", "--match-set", string(dstHost.ipsetName), "dst")
		dstComment = fmt.Sprintf("pods: namespace: %s, selector: %s", dstHost.nsName, dstHost.key)
	}
	if dstPort != nil {
		args = append(args, "--dport", *dstPort)
	}
	args = append(args, "-j", "ACCEPT")
	args = append(args, "-m", "comment", "--comment", fmt.Sprintf("%s -> %s", srcComment, dstComment))
	key := strings.Join(args, " ")

	return &ruleSpec{key, args, true}
}

func newRuleSpecDeny(dstHost *selectorSpec) *ruleSpec {
	comment := fmt.Sprintf("deny to pods: namespace: %s, selector: %s", dstHost.nsName, dstHost.key)
	args := []string{
		"-m", "set", "--match-set", string(dstHost.ipsetName), "dst",
		"-j", IngressDropChain,
		"-m", "comment", "--comment", comment,
	}
	key := strings.Join(args, " ")

	return &ruleSpec{key, args, false}
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
				chain := IngressChain
				if !spec.allow {
					chain = IngressIsolateChain
				}
				common.Log.Infof("deleting rule: %v, chain: %q", spec.args, chain)
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
				chain := IngressChain
				if !spec.allow {
					chain = IngressIsolateChain
				}
				common.Log.Infof("adding rule: %v, chain: %q", spec.args, chain)
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
