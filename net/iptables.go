package net

import (
	"strings"
	"syscall"
	"time"

	"github.com/coreos/go-iptables/iptables"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	utilwait "k8s.io/apimachinery/pkg/util/wait"
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

func chainExists(ipt *iptables.IPTables, table string, chain string) (bool, error) {
	existingChains, err := ipt.ListChains(table)
	if err != nil {
		return false, errors.Wrapf(err, "ipt.ListChains(%s)", table)
	}
	chainMap := make(map[string]struct{})
	for _, c := range existingChains {
		chainMap[c] = struct{}{}
	}

	_, found := chainMap[chain]
	return found, nil
}

const (
	// Max time we wait for an iptables flush to complete after we notice it has started
	iptablesFlushTimeout = 5 * time.Second
	// How often we poll while waiting for an iptables flush to complete
	iptablesFlushPollTime = 100 * time.Millisecond
)

// MonitorForIptablesFlush periodically checks for a canary chain in iptables. If this canary chain goes missing it calls the reloadFunc.
// This is a more efficient way of detecting whether firewalld or another process has been removing rules that we rely on.
// The reloadFunc can then check whether other chains that should exist are still there, fix things and restore the canary.
func MonitorForIptablesFlush(log *logrus.Logger, canary string, tables []string, reloadFunc func(), interval time.Duration, stopCh <-chan struct{}) {
	ipt, err := iptables.New()
	if err != nil {
		log.Errorf("creating iptables object while initializing iptable Monitoring %s", err)
		return
	}

	for {
		_ = PollImmediateUntil(interval, func() (bool, error) {
			for _, table := range tables {
				if err := ensureChains(ipt, table, canary); err != nil {
					log.Warningf("Could not set up iptables canary %s/%s: %v", table, canary, err)
					return false, nil
				}
			}
			return true, nil
		}, stopCh)

		// Poll until stopCh is closed or iptables is flushed
		err = utilwait.PollUntil(interval, func() (bool, error) {
			if exists, err := chainExists(ipt, tables[0], canary); exists {
				return false, nil
			} else if isResourceError(err) {
				log.Warningf("Could not check for iptables canary %s/%s: %v", tables[0], canary, err)
				return false, nil
			}
			log.Infof("iptables canary %s/%s deleted", tables[0], canary)

			// Wait for the other canaries to be deleted too before returning
			// so we don't start reloading too soon.
			err := utilwait.PollImmediate(iptablesFlushPollTime, iptablesFlushTimeout, func() (bool, error) {
				for i := 1; i < len(tables); i++ {
					if exists, err := chainExists(ipt, tables[i], canary); exists || isResourceError(err) {
						return false, nil
					}
				}
				return true, nil
			})
			if err != nil {
				log.Warning("Inconsistent iptables state detected.")
			}
			return true, nil
		}, stopCh)

		if err != nil {
			// stopCh was closed
			for _, table := range tables {
				_ = ipt.DeleteChain(table, canary)
			}
			return
		}

		log.Infof("Reloading after iptables flush")
		reloadFunc()
	}
}

const iptablesStatusResourceProblem = 4

// isResourceError returns true if the error indicates that iptables ran into a "resource
// problem" and was unable to attempt the request. In particular, this will be true if it
// times out trying to get the iptables lock.
func isResourceError(err error) bool {
	if ierr, ok := err.(*iptables.Error); ok {
		if status, ok := ierr.ExitError.Sys().(syscall.WaitStatus); ok {
			return status.ExitStatus() == iptablesStatusResourceProblem
		}
	}

	return false
}

// PollImmediateUntil tries a condition func until it returns true, an error or stopCh is closed.
//
// PollImmediateUntil runs the 'condition' before waiting for the interval.
// 'condition' will always be invoked at least once.
func PollImmediateUntil(interval time.Duration, condition utilwait.ConditionFunc, stopCh <-chan struct{}) error {
	done, err := condition()
	if err != nil {
		return err
	}
	if done {
		return nil
	}
	select {
	case <-stopCh:
		return utilwait.ErrWaitTimeout
	default:
		return utilwait.PollUntil(interval, condition, stopCh)
	}
}
