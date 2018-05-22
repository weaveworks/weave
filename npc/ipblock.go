package npc

import (
	"sort"
	"strings"

	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/npc/ipset"
)

type ipBlockSpec struct {
	key       string
	ipsetName ipset.Name // ipset for storing excepted CIDRs
	ipBlock   *networkingv1.IPBlock
}

func newIPBlockSpec(ipb *networkingv1.IPBlock) *ipBlockSpec {
	spec := &ipBlockSpec{ipBlock: ipb}

	if len(ipb.Except) > 0 {
		sort.Strings(ipb.Except)
		spec.key = strings.Join(ipb.Except, " ")
		spec.ipsetName = ipset.Name(IpsetNamePrefix + shortName("ipblock-except:"+spec.key))
	}

	return spec
}

//func (ipb ipBlock) GetRuleSpec() (spec *ruleSpec) {
//	spec = &ruleSpec{
//		chain: IngressIPBlockChain,
//	}
//
//	if len(ipb.spec.Except) == 0 {
//		spec.args = append(spec.args, "-s", ipb.spec.CIDR)
//		spec.comment = fmt.Sprintf("cidr: %s", ipb.spec.CIDR)
//	} else {
//		spec.args = append(spec.args, "-s", ipb.spec.CIDR, "-m", "set", "!", "--match-set",
//			string(ipb.ipsetName), "src")
//		spec.comment = fmt.Sprintf("cidr: %s except [%s]", ipb.spec.CIDR,
//			strings.Join(ipb.spec.Except, `,`))
//	}
//	return
//}

type ipBlockSet struct {
	ips   ipset.Interface
	users map[string]map[types.UID]struct{}
}

func newIPBlockSet(ips ipset.Interface) *ipBlockSet {
	return &ipBlockSet{
		ips:   ips,
		users: make(map[string]map[types.UID]struct{}),
	}
}

func (s *ipBlockSet) deprovision(user types.UID, current, desired map[string]*ipBlockSpec) error {
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

func (s *ipBlockSet) provision(user types.UID, current, desired map[string]*ipBlockSpec) (err error) {
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

				s.users[key] = make(map[types.UID]struct{})
			}
			s.users[key][user] = struct{}{}
		}
	}

	return nil
}
