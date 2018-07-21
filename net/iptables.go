package net

import (
	"github.com/coreos/go-iptables/iptables"
	"github.com/pkg/errors"
)

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
