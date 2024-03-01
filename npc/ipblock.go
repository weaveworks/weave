package npc

import (
	"fmt"
	"sort"
	"strings"

	networkingv1 "k8s.io/api/networking/v1"

	"github.com/rajch/weave/common"
	"github.com/rajch/weave/net/ipset"
)

type ipBlockSpec struct {
	key       string
	ipsetName ipset.Name // ipset for storing excepted CIDRs
	ipBlock   *networkingv1.IPBlock
	nsName    string // Namespace name
}

func newIPBlockSpec(ipb *networkingv1.IPBlock, nsName string) *ipBlockSpec {
	spec := &ipBlockSpec{ipBlock: ipb, nsName: nsName}

	if len(ipb.Except) > 0 {
		sort.Strings(ipb.Except)
		spec.key = strings.Join(ipb.Except, " ")
		spec.ipsetName = ipset.Name(IpsetNamePrefix + shortName(nsName+":"+"ipblock-except:"+spec.key))
	}

	return spec
}

func (spec *ipBlockSpec) getRuleSpec(src bool) ([]string, string) {
	dir, dirOpt := "dst", "-d"
	if src {
		dir, dirOpt = "src", "-s"
	}

	if spec.key == "" {
		rule := []string{dirOpt, spec.ipBlock.CIDR}
		comment := fmt.Sprintf("cidr: %s", spec.ipBlock.CIDR)
		return rule, comment
	}

	rule := []string{dirOpt, spec.ipBlock.CIDR,
		"-m", "set", "!", "--match-set", string(spec.ipsetName), dir}
	comment := fmt.Sprintf("cidr: %s except [%s]", spec.ipBlock.CIDR, spec.key)
	return rule, comment
}

type ipBlockSet struct {
	ips   ipset.Interface
	users map[string]map[ipset.UID]struct{}
}

func newIPBlockSet(ips ipset.Interface) *ipBlockSet {
	return &ipBlockSet{
		ips:   ips,
		users: make(map[string]map[ipset.UID]struct{}),
	}
}

func (s *ipBlockSet) deprovision(user ipset.UID, current, desired map[string]*ipBlockSpec) error {
	for key, spec := range current {
		if key == "" {
			continue
		}

		if _, found := desired[key]; !found {
			delete(s.users[key], user)
			if len(s.users[key]) == 0 {
				common.Log.Infof("destroying ipset: %#v", spec)
				if err := s.ips.Destroy(spec.ipsetName); err != nil {
					return err
				}

				delete(s.users, key)
			}
		}
	}

	return nil
}

func (s *ipBlockSet) provision(user ipset.UID, current, desired map[string]*ipBlockSpec) (err error) {
	for key, spec := range desired {
		if key == "" {
			// No need to provision an ipBlock with empty list of excepted CIDRs
			// (i.e. no ipset is needed for the related iptables rule).
			continue
		}

		if _, found := current[key]; !found {
			if _, found := s.users[key]; !found {
				common.Log.Infof("creating ipset: %#v", spec)
				if err := s.ips.Create(spec.ipsetName, ipset.HashNet); err != nil {
					return err
				}

				// TODO(brb) Pass comment to ips.Create instead.
				comment := "excepted from " + spec.ipBlock.CIDR
				for _, cidr := range spec.ipBlock.Except {
					if err = s.ips.AddEntry(user, spec.ipsetName, cidr, comment); err != nil {
						return err
					}
				}

				s.users[key] = make(map[ipset.UID]struct{})
			}
			s.users[key][user] = struct{}{}
		}
	}

	return nil
}
