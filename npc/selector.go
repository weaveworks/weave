package npc

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"

	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/net/ipset"
)

type selectorSpec struct {
	key      string          // string representation (for hash keying/equality comparison)
	selector labels.Selector // k8s Selector object (for matching)
	dst      bool            // destination selector

	ipsetType ipset.Type // type of ipset to provision
	ipsetName ipset.Name // generated ipset name
	nsName    string     // Namespace name
}

func newSelectorSpec(json *metav1.LabelSelector, dst bool, nsName string, ipsetType ipset.Type) (*selectorSpec, error) {
	selector, err := metav1.LabelSelectorAsSelector(json)
	if err != nil {
		return nil, err
	}
	key := selector.String()
	return &selectorSpec{
		key:      key,
		selector: selector,
		dst:      dst,
		// We prefix the selector string with the namespace name when generating
		// the shortname because you can specify the same selector in multiple
		// namespaces - we need those to map to distinct ipsets
		ipsetName: ipset.Name(IpsetNamePrefix + shortName(nsName+":"+key)),
		ipsetType: ipsetType,
		nsName:    nsName}, nil
}

type selector struct {
	ips  ipset.Interface
	spec *selectorSpec
}

func (s *selector) matches(labelMap map[string]string) bool {
	return s.spec.selector.Matches(labels.Set(labelMap))
}

func (s *selector) addEntry(user types.UID, entry string, comment string) error {
	return s.ips.AddEntry(user, s.spec.ipsetName, entry, comment)
}

func (s *selector) delEntry(user types.UID, entry string) error {
	return s.ips.DelEntry(user, s.spec.ipsetName, entry)
}

type selectorFn func(selector *selector) error

type selectorSet struct {
	ips           ipset.Interface
	onNewSelector selectorFn

	// invoked after dst selector has been provisioned for the first time
	onNewDstSelector selectorFn
	// invoked after the last instance of dst selector has been deprovisioned
	onDestroyDstSelector selectorFn

	users   map[string]map[types.UID]struct{} // list of users per selector
	entries map[string]*selector

	// We need to keep track of dst selector instances to be able to invoke
	// onNewDstSelector and onDestroyDstSelector callbacks at the right time;
	// selectorSpec.Key -> count
	dstSelectorsCount map[string]int
}

func newSelectorSet(ips ipset.Interface, onNewSelector, onNewDstSelector selectorFn, onDestroyDstSelector selectorFn) *selectorSet {
	return &selectorSet{
		ips:                  ips,
		onNewSelector:        onNewSelector,
		onNewDstSelector:     onNewDstSelector,
		onDestroyDstSelector: onDestroyDstSelector,
		users:                make(map[string]map[types.UID]struct{}),
		entries:              make(map[string]*selector),
		dstSelectorsCount:    make(map[string]int)}
}

func (ss *selectorSet) addToMatching(user types.UID, labelMap map[string]string, entry string, comment string) (bool, error) {
	found := false
	for _, s := range ss.entries {
		if s.matches(labelMap) {
			if ss.dstSelectorExist(s) {
				found = true
			}
			if err := s.addEntry(user, entry, comment); err != nil {
				return found, err
			}
		}
	}
	return found, nil
}

func (ss *selectorSet) delFromMatching(user types.UID, labelMap map[string]string, entry string) error {
	for _, s := range ss.entries {
		if s.matches(labelMap) {
			if err := s.delEntry(user, entry); err != nil {
				return err
			}
		}
	}
	return nil
}

func (ss *selectorSet) dstSelectorExist(s *selector) bool {
	return ss.dstSelectorsCount[s.spec.key] > 0
}

func (ss *selectorSet) deprovision(user types.UID, current, desired map[string]*selectorSpec) error {
	for key, spec := range current {
		if _, found := desired[key]; !found {
			delete(ss.users[key], user)
			if len(ss.users[key]) == 0 {
				common.Log.Infof("destroying ipset: %#v", spec)
				if err := ss.ips.Destroy(spec.ipsetName); err != nil {
					return err
				}

				delete(ss.entries, key)
				delete(ss.users, key)
			}

			if spec.dst {
				ss.dstSelectorsCount[key]--
				if ss.dstSelectorsCount[key] == 0 {
					if err := ss.onDestroyDstSelector(&selector{ss.ips, spec}); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func (ss *selectorSet) provision(user types.UID, current, desired map[string]*selectorSpec) error {
	for key, spec := range desired {
		if _, found := current[key]; !found {
			selector := &selector{ss.ips, spec}

			if _, found := ss.users[key]; !found {
				common.Log.Infof("creating ipset: %#v", spec)
				if err := ss.ips.Create(spec.ipsetName, spec.ipsetType); err != nil {
					return err
				}
				if err := ss.onNewSelector(selector); err != nil {
					return err
				}
				ss.users[key] = make(map[types.UID]struct{})
				ss.entries[key] = selector
			}
			ss.users[key][user] = struct{}{}

			if spec.dst {
				ss.dstSelectorsCount[key]++
				if ss.dstSelectorsCount[key] == 1 {
					if err := ss.onNewDstSelector(selector); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}
