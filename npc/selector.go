package npc

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/rajch/weave/common"
	"github.com/rajch/weave/net/ipset"
)

type selectorSpec struct {
	key               string          // string representation (for hash keying/equality comparison)
	podSelector       labels.Selector // k8s Selector object (for matching pods)
	namespaceSelector labels.Selector // k8s Selector object (for matching namespaces)
	policyTypes       []policyType    // If non-empty, then selectorSpec is a target selector for given policyTypes.

	ipsetType ipset.Type // type of ipset to provision
	ipsetName ipset.Name // generated ipset name
	nsName    string     // Namespace name
}

func newSelectorSpec(podJSON *metav1.LabelSelector, namespaceJSON *metav1.LabelSelector, policyType []policyType, nsName string, ipsetType ipset.Type) (*selectorSpec, error) {
	var podSelector, namespaceSelector labels.Selector
	var err error
	var key string
	if namespaceJSON != nil {
		namespaceSelector, err = metav1.LabelSelectorAsSelector(namespaceJSON)
		if err != nil {
			return nil, err
		}
		key = namespaceSelector.String()
	}
	if podJSON != nil {
		podSelector, err = metav1.LabelSelectorAsSelector(podJSON)
		if err != nil {
			return nil, err
		}
		key = key + podSelector.String()
	}
	return &selectorSpec{
		key:               key,
		podSelector:       podSelector,
		namespaceSelector: namespaceSelector,
		policyTypes:       policyType,
		// We prefix the selector string with the namespace name when generating
		// the shortname because you can specify the same selector in multiple
		// namespaces - we need those to map to distinct ipsets
		ipsetName: ipset.Name(IpsetNamePrefix + shortName(nsName+":"+key)),
		ipsetType: ipsetType,
		nsName:    nsName}, nil
}

func (spec *selectorSpec) getRuleSpec(src bool) ([]string, string) {
	dir := "dst"
	if src {
		dir = "src"
	}
	rule := []string{"-m", "set", "--match-set", string(spec.ipsetName), dir}

	var comment string
	if spec.nsName != "" {
		comment = fmt.Sprintf("pods: namespace: %s, selector: %s", spec.nsName, spec.key)
	} else {
		comment = fmt.Sprintf("namespaces: selector: %s", spec.key)
	}

	return rule, comment
}

type selector struct {
	ips  ipset.Interface
	spec *selectorSpec
}

func (s *selector) matchesPodSelector(podsLabelMap map[string]string) bool {
	return s.spec.podSelector.Matches(labels.Set(podsLabelMap))
}

func (s *selector) matchesNamespaceSelector(namespaceLabelMap map[string]string) bool {
	return s.spec.namespaceSelector.Matches(labels.Set(namespaceLabelMap))
}

func (s *selector) matchesNamespacedPodSelector(podsLabelMap, namespaceLabelMap map[string]string) bool {
	return s.spec.podSelector.Matches(labels.Set(podsLabelMap)) && s.spec.namespaceSelector.Matches(labels.Set(namespaceLabelMap))
}

func (s *selector) addEntry(user ipset.UID, entry string, comment string) error {
	return s.ips.AddEntry(user, s.spec.ipsetName, entry, comment)
}

func (s *selector) delEntry(user ipset.UID, entry string) error {
	return s.ips.DelEntry(user, s.spec.ipsetName, entry)
}

type selectorFn func(selector *selector) error
type selectorWithPolicyTypeFn func(selector *selector, policyType policyType) error

type selectorSet struct {
	ips           ipset.Interface
	onNewSelector selectorFn

	// invoked after target selector has been provisioned for the first time
	onNewTargetSelector selectorWithPolicyTypeFn
	// invoked after the last instance of target selector has been deprovisioned
	onDestroyTargetSelector selectorWithPolicyTypeFn

	users   map[string]map[ipset.UID]struct{} // list of users per selector
	entries map[string]*selector

	// We need to keep track of target selector instances to be able to invoke
	// onNewTargetSelector and onDestroyTargetSelector callbacks at the right time;
	// selectorSpec.Key -> policyType -> count
	targetSelectorsCount map[string]map[policyType]int
}

func newSelectorSet(ips ipset.Interface, onNewSelector selectorFn, onNewTargetSelector, onDestroyTargetSelector selectorWithPolicyTypeFn) *selectorSet {
	return &selectorSet{
		ips:                     ips,
		onNewSelector:           onNewSelector,
		onNewTargetSelector:     onNewTargetSelector,
		onDestroyTargetSelector: onDestroyTargetSelector,
		users:                   make(map[string]map[ipset.UID]struct{}),
		entries:                 make(map[string]*selector),
		targetSelectorsCount:    make(map[string]map[policyType]int)}
}

func (ss *selectorSet) addToMatchingPodSelector(user ipset.UID, podLabelsMap map[string]string, entry string, comment string) (bool, bool, error) {
	foundIngress := false
	foundEgress := false
	for _, s := range ss.entries {
		if s.matchesPodSelector(podLabelsMap) {
			if ss.targetSelectorExist(s, policyTypeIngress) {
				foundIngress = true
			}
			if ss.targetSelectorExist(s, policyTypeEgress) {
				foundEgress = true
			}
			if err := s.addEntry(user, entry, comment); err != nil {
				return foundIngress, foundEgress, err
			}
		}
	}
	return foundIngress, foundEgress, nil
}

func (ss *selectorSet) addToMatchingNamespaceSelector(user ipset.UID, namespaceLabelsMap map[string]string, entry string, comment string) error {
	for _, s := range ss.entries {
		if s.matchesNamespaceSelector(namespaceLabelsMap) {
			if err := s.addEntry(user, entry, comment); err != nil {
				return err
			}
		}
	}
	return nil
}

func (ss *selectorSet) addToMatchingNamespacedPodSelector(user ipset.UID, podLabelsMap map[string]string, namespaceLabelsMap map[string]string, entry string, comment string) error {
	for _, s := range ss.entries {
		if s.matchesNamespacedPodSelector(podLabelsMap, namespaceLabelsMap) {
			if err := s.addEntry(user, entry, comment); err != nil {
				return err
			}
		}
	}
	return nil
}

func (ss *selectorSet) delFromMatchingPodSelector(user ipset.UID, podLabelsMap map[string]string, entry string) error {
	for _, s := range ss.entries {
		if s.matchesPodSelector(podLabelsMap) {
			if err := s.delEntry(user, entry); err != nil {
				return err
			}
		}
	}
	return nil
}

func (ss *selectorSet) delFromMatchingNamespaceSelector(user ipset.UID, namespaceLabelsMap map[string]string, entry string) error {
	for _, s := range ss.entries {
		if s.matchesNamespaceSelector(namespaceLabelsMap) {
			if err := s.delEntry(user, entry); err != nil {
				return err
			}
		}
	}
	return nil
}

func (ss *selectorSet) delFromMatchingNamespacedPodSelector(user ipset.UID, podLabelsMap map[string]string, namespaceLabelsMap map[string]string, entry string) error {
	for _, s := range ss.entries {
		if s.matchesNamespacedPodSelector(podLabelsMap, namespaceLabelsMap) {
			if err := s.delEntry(user, entry); err != nil {
				return err
			}
		}
	}
	return nil
}

func (ss *selectorSet) targetSelectorExist(s *selector, policyType policyType) bool {
	return ss.targetSelectorsCount[s.spec.key][policyType] > 0
}

func (ss *selectorSet) deprovision(user ipset.UID, current, desired map[string]*selectorSpec) error {
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

			for _, policyType := range spec.policyTypes {
				ss.targetSelectorsCount[key][policyType]--
				if ss.targetSelectorsCount[key][policyType] == 0 {
					if err := ss.onDestroyTargetSelector(&selector{ss.ips, spec}, policyType); err != nil {
						return err
					}
				}
				// TODO(brb) delete(...)
			}
		}
	}
	return nil
}

func (ss *selectorSet) provision(user ipset.UID, current, desired map[string]*selectorSpec) error {
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
				ss.users[key] = make(map[ipset.UID]struct{})
				ss.entries[key] = selector
			}
			ss.users[key][user] = struct{}{}

			for _, pt := range spec.policyTypes {
				if _, found := ss.targetSelectorsCount[key]; !found {
					ss.targetSelectorsCount[key] = make(map[policyType]int)
				}
				ss.targetSelectorsCount[key][pt]++
				if ss.targetSelectorsCount[key][pt] == 1 {
					if err := ss.onNewTargetSelector(selector, pt); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}
