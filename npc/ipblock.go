package npc

import (
	"fmt"
	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/npc/ipset"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"net"
	"sort"
	"strings"
)

type ipBlock struct {
	ipsetName ipset.Name // generated ipset name
	spec      *networkingv1.IPBlock
}

func newIPBlock(ipb *networkingv1.IPBlock, ns string) (b *ipBlock, err error) {
	if _, _, err = net.ParseCIDR(ipb.CIDR); err != nil {
		return
	}

	for _, ex := range ipb.Except {
		if _, _, err = net.ParseCIDR(ex); err != nil {
			return
		}
	}

	b = &ipBlock{spec: ipb}

	if len(ipb.Except) > 0 {
		sortedExcept := ipb.Except
		sort.Strings(sortedExcept)
		b.ipsetName = ipset.Name(IpsetNamePrefix + shortName("except:"+strings.Join(sortedExcept, "")))
	}

	return
}

func (ipb ipBlock) GetRuleArgs() (args []string, comment string) {
	if len(ipb.spec.Except) == 0 {
		args = append(args, "-s", ipb.spec.CIDR)
		comment = fmt.Sprintf("cidr: %s", ipb.spec.CIDR)
	} else {
		args = append(args, "-s", ipb.spec.CIDR, "-m", "set", "!", "--match-set", string(ipb.ipsetName), "src")
		comment = fmt.Sprintf("cidr: %s except [%s]", ipb.spec.CIDR, strings.Join(ipb.spec.Except, `,`))
	}
	return
}

type exceptedIPBlockSet struct {
	ips   ipset.Interface
	users map[string]int
}

func newExceptedIPBlockSet(ips ipset.Interface) *exceptedIPBlockSet {
	return &exceptedIPBlockSet{
		ips:   ips,
		users: make(map[string]int),
	}
}

func (s *exceptedIPBlockSet) deprovision(uid types.UID, current *ipBlock) (err error) {
	if current == nil || len(current.ipsetName) == 0 {
		return
	}

	key := string(current.ipsetName)
	if _, found := s.users[key]; !found {
		return
	}

	s.users[key]--
	if s.users[key] == 0 {
		if err = s.ips.Destroy(current.ipsetName); err != nil {
			return
		}

		delete(s.users, key)
	}

	return
}

func (s *exceptedIPBlockSet) provision(uid types.UID, desired *ipBlock) (err error) {
	if desired == nil || len(desired.ipsetName) == 0 {
		return
	}

	key := string(desired.ipsetName)
	if _, found := s.users[key]; found {
		common.Log.Debug()
		s.users[key]++
		return
	}

	err = s.ips.Create(desired.ipsetName, ipset.HashNet)
	if err != nil {
		return
	}

	comment := `excepted from ` + desired.spec.CIDR
	for _, excepted := range desired.spec.Except {
		if err = s.ips.AddEntry(uid, desired.ipsetName, excepted, comment); err != nil {
			if details := s.ips.Destroy(desired.ipsetName); details != nil {
				common.Log.Warn("can't destroy ipset ", desired.ipsetName, ":", details)
			}
			return
		}
	}

	s.users[key] = 1
	return
}
