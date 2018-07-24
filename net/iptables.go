package net

import (
	"strings"

	"github.com/coreos/go-iptables/iptables"
	"github.com/pkg/errors"
)

// AddChainWithRules creates a chain and appends given rules to it.
//
// If the chain exists, but its rules are not the same as the given ones, the
// function will flush the chain and then will append the rules.
func AddChainWithRules(ipt *iptables.IPTables, table, chain string, rulespecs [][]string) error {
	if err := ensureChains(ipt, table, chain); err != nil {
		return err
	}

	currRuleSpecs, err := ipt.List(table, chain)
	if err != nil {
		return errors.Wrapf(err, "iptables -S. table: %q, chain: %q", table, chain)
	}

	// First returned rule is "-N $(chain)", so ignore it
	currRules := strings.Join(currRuleSpecs[1:], "\n")
	rules := make([]string, 0)
	for _, r := range rulespecs {
		rules = append(rules, strings.Join(r, " "))
	}
	reqRules := strings.Join(rules, "\n")

	if currRules == reqRules {
		return nil
	}

	if err := ipt.ClearChain(table, chain); err != nil {
		return err
	}

	for _, r := range rulespecs {
		if err := ipt.Append(table, chain, r...); err != nil {
			return errors.Wrapf(err, "iptables -A. table: %q, chain: %q, rule: %s", table, chain, r)
		}
	}

	return nil
}

// ensureChains creates given chains if they do not exist.
func ensureChains(ipt *iptables.IPTables, table string, chains ...string) error {
	existingChains, err := ipt.ListChains(table)
	if err != nil {
		return errors.Wrapf(err, "ipt.ListChains(%s)", table)
	}
	chainMap := make(map[string]struct{})
	for _, c := range existingChains {
		chainMap[c] = struct{}{}
	}

	for _, c := range chains {
		if _, found := chainMap[c]; !found {
			if err := ipt.NewChain(table, c); err != nil {
				return errors.Wrapf(err, "ipt.NewChain(%s, %s)", table, c)
			}
		}
	}

	return nil
}

// ensureRulesAtTop ensures the presence of given iptables rules.
//
// If any rule from the list is missing, the function deletes all given
// rules and re-inserts them at the top of the chain to ensure the order of the rules.
func ensureRulesAtTop(table, chain string, rulespecs [][]string, ipt *iptables.IPTables) error {
	allFound := true

	for _, rs := range rulespecs {
		found, err := ipt.Exists(table, chain, rs...)
		if err != nil {
			return errors.Wrapf(err, "ipt.Exists(%s, %s, %s)", table, chain, rs)
		}
		if !found {
			allFound = false
			break
		}
	}

	// All rules exist, do nothing.
	if allFound {
		return nil
	}

	for pos, rs := range rulespecs {
		// If any is missing, then delete all, as we need to preserve the order of
		// given rules. Ignore errors, as rule might not exist.
		if !allFound {
			ipt.Delete(table, chain, rs...)
		}
		if err := ipt.Insert(table, chain, pos+1, rs...); err != nil {
			return errors.Wrapf(err, "ipt.Append(%s, %s, %s)", table, chain, rs)
		}
	}

	return nil
}
