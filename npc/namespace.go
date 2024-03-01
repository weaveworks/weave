package npc

import (
	"errors"
	"fmt"

	coreapi "k8s.io/api/core/v1"
	extnapi "k8s.io/api/extensions/v1beta1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"

	"github.com/rajch/weave/common"
	"github.com/rajch/weave/common/chains"
	"github.com/rajch/weave/net/ipset"
	"github.com/rajch/weave/npc/iptables"
)

var errInvalidNetworkPolicyObjType = errors.New("invalid NetworkPolicy object type")

type ns struct {
	ipt iptables.Interface // interface to iptables
	ips ipset.Interface    // interface to ipset

	name            string                     // k8s Namespace name
	nodeName        string                     // my node name
	namespaceUID    ipset.UID                  // k8s Namespace UID
	namespaceLabels map[string]string          // k8s Namespace labels
	pods            map[ipset.UID]*coreapi.Pod // k8s Pod objects by UID
	policies        map[ipset.UID]interface{}  // k8s NetworkPolicy objects by UID

	uid     ipset.UID     // surrogate UID to own allPods selector
	allPods *selectorSpec // hash:ip ipset of all pod IPs in this namespace

	// stores IP addrs of pods which are not selected by any target podSelector of
	// any netpol; used as a target in the WEAVE-NPC-{INGRESS,EGRESS}-DEFAULT
	// iptables chains.
	ingressDefaultAllowIPSet ipset.Name
	egressDefaultAllowIPSet  ipset.Name

	nsSelectors             *selectorSet // reference to global selectorSet that is shared across the `ns`. Used to represent all pods in the matching namespaces
	podSelectors            *selectorSet // used to represent the matching pods in namespace respresented by this `ns`
	namespacedPodsSelectors *selectorSet // reference to global selectorSet that is shared across the `ns`. Used to represent matching pods in matching namespace
	ipBlocks                *ipBlockSet
	rules                   *ruleSet
}

func newNS(name, nodeName string, ipt iptables.Interface, ips ipset.Interface, nsSelectors *selectorSet, namespacedPodsSelectors *selectorSet, namespaceObj *coreapi.Namespace) (*ns, error) {
	allPods, err := newSelectorSpec(&metav1.LabelSelector{}, nil, nil, name, ipset.HashIP)
	if err != nil {
		return nil, err
	}

	ns := &ns{
		ipt:                     ipt,
		ips:                     ips,
		name:                    name,
		namespaceUID:            nsuid(namespaceObj),
		namespaceLabels:         namespaceObj.ObjectMeta.Labels,
		nodeName:                nodeName,
		pods:                    make(map[ipset.UID]*coreapi.Pod),
		policies:                make(map[ipset.UID]interface{}),
		uid:                     ipset.UID(uuid.NewUUID()),
		allPods:                 allPods,
		nsSelectors:             nsSelectors,
		namespacedPodsSelectors: namespacedPodsSelectors,
		ipBlocks:                newIPBlockSet(ips),
		rules:                   newRuleSet(ipt),
	}

	ns.podSelectors = newSelectorSet(ips, ns.onNewPodSelector, ns.onNewTargetPodSelector, ns.onDestroyTargetPodSelector)

	ingressDefaultAllowIPSet := ipset.Name(IpsetNamePrefix + shortName("ingress-default-allow:"+name))
	if err := ips.Create(ingressDefaultAllowIPSet, ipset.HashIP); err != nil {
		return nil, err
	}
	ns.ingressDefaultAllowIPSet = ingressDefaultAllowIPSet

	egressDefaultAllowIPSet := ipset.Name(IpsetNamePrefix + shortName("egress-default-allow:"+name))
	if err := ips.Create(egressDefaultAllowIPSet, ipset.HashIP); err != nil {
		return nil, err
	}
	ns.egressDefaultAllowIPSet = egressDefaultAllowIPSet

	if err := ns.podSelectors.provision(ns.uid, nil, map[string]*selectorSpec{ns.allPods.key: ns.allPods}); err != nil {
		return nil, err
	}

	return ns, nil
}

func (ns *ns) empty() bool {
	return len(ns.pods) == 0 && len(ns.policies) == 0 && ns.namespaceUID == ""
}

func (ns *ns) destroy() error {
	return ns.podSelectors.deprovision(ns.uid, map[string]*selectorSpec{ns.allPods.key: ns.allPods}, nil)
}

func (ns *ns) onNewPodSelector(selector *selector) error {
	for _, pod := range ns.pods {
		if hasIP(pod) {
			if selector.matchesPodSelector(pod.ObjectMeta.Labels) {
				if err := selector.addEntry(uid(pod), pod.Status.PodIP, podComment(pod)); err != nil {
					return err
				}

			}
		}
	}
	return nil
}

func (ns *ns) onNewTargetPodSelector(selector *selector, policyType policyType) error {
	for _, pod := range ns.pods {
		if hasIP(pod) {
			// Remove the pod from default-allow if dst podselector matches the pod
			ipset := ns.defaultAllowIPSetName(policyType)
			if selector.matchesPodSelector(pod.ObjectMeta.Labels) {
				if err := ns.ips.DelEntry(uid(pod), ipset, pod.Status.PodIP); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (ns *ns) onDestroyTargetPodSelector(selector *selector, policyType policyType) error {
	for _, pod := range ns.pods {
		if hasIP(pod) {
			if selector.matchesPodSelector(pod.ObjectMeta.Labels) {
				if err := ns.addToDefaultAllowIfNoMatching(pod, policyType); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// Add pod IP addr to default-allow ipset if there are no matching target selectors
func (ns *ns) addToDefaultAllowIfNoMatching(pod *coreapi.Pod, policyType policyType) error {
	found := false
	// TODO(mp) optimize (avoid iterating over selectors) by ref counting IP addrs.
	for _, s := range ns.podSelectors.entries {
		if ns.podSelectors.targetSelectorExist(s, policyType) && s.matchesPodSelector(pod.ObjectMeta.Labels) {
			found = true
			break
		}
	}
	if !found {
		ipset := ns.defaultAllowIPSetName(policyType)
		if err := ns.ips.AddEntry(uid(pod), ipset, pod.Status.PodIP, podComment(pod)); err != nil {
			return err
		}
	}
	return nil
}

func (ns *ns) addPod(obj *coreapi.Pod) error {
	ns.pods[uid(obj)] = obj

	if !hasIP(obj) {
		return nil
	}

	foundIngress, foundEgress, err := ns.podSelectors.addToMatchingPodSelector(uid(obj), obj.ObjectMeta.Labels, obj.Status.PodIP, podComment(obj))
	if err != nil {
		return err
	}
	// If there are no matching target selectors, add the pod to default-allow
	if !foundIngress {
		if err := ns.ips.AddEntry(uid(obj), ns.ingressDefaultAllowIPSet, obj.Status.PodIP, podComment(obj)); err != nil {
			return err
		}
	}
	if !foundEgress {
		if err := ns.ips.AddEntry(uid(obj), ns.egressDefaultAllowIPSet, obj.Status.PodIP, podComment(obj)); err != nil {
			return err
		}
	}

	err = ns.namespacedPodsSelectors.addToMatchingNamespacedPodSelector(uid(obj), obj.ObjectMeta.Labels, ns.namespaceLabels, obj.Status.PodIP, podComment(obj))
	if err != nil {
		return err
	}

	return nil
}

func (ns *ns) updatePod(oldObj, newObj *coreapi.Pod) error {
	delete(ns.pods, uid(oldObj))
	ns.pods[uid(newObj)] = newObj

	if !hasIP(oldObj) && !hasIP(newObj) {
		return nil
	}

	if hasIP(oldObj) && !hasIP(newObj) {
		if err := ns.ips.DelEntry(uid(oldObj), ns.ingressDefaultAllowIPSet, oldObj.Status.PodIP); err != nil {
			return err
		}
		if err := ns.ips.DelEntry(uid(oldObj), ns.egressDefaultAllowIPSet, oldObj.Status.PodIP); err != nil {
			return err
		}
		if err := ns.namespacedPodsSelectors.delFromMatchingNamespacedPodSelector(uid(oldObj), oldObj.ObjectMeta.Labels, ns.namespaceLabels, oldObj.Status.PodIP); err != nil {
			return err
		}

		return ns.podSelectors.delFromMatchingPodSelector(uid(oldObj), oldObj.ObjectMeta.Labels, oldObj.Status.PodIP)
	}

	if !hasIP(oldObj) && hasIP(newObj) {
		foundIngress, foundEgress, err := ns.podSelectors.addToMatchingPodSelector(uid(newObj), newObj.ObjectMeta.Labels, newObj.Status.PodIP, podComment(newObj))
		if err != nil {
			return err
		}

		if !foundIngress {
			if err := ns.ips.AddEntry(uid(newObj), ns.ingressDefaultAllowIPSet, newObj.Status.PodIP, podComment(newObj)); err != nil {
				return err
			}
		}
		if !foundEgress {
			if err := ns.ips.AddEntry(uid(newObj), ns.egressDefaultAllowIPSet, newObj.Status.PodIP, podComment(newObj)); err != nil {
				return err
			}
		}
		err = ns.namespacedPodsSelectors.addToMatchingNamespacedPodSelector(uid(newObj), newObj.ObjectMeta.Labels, ns.namespaceLabels, newObj.Status.PodIP, podComment(newObj))
		if err != nil {
			return err
		}

		return nil
	}

	if oldObj.Status.PodIP != newObj.Status.PodIP {
		if err := ns.updateDefaultAllowIPSetEntry(oldObj, newObj, ns.ingressDefaultAllowIPSet); err != nil {
			return err
		}
		if err := ns.updateDefaultAllowIPSetEntry(oldObj, newObj, ns.egressDefaultAllowIPSet); err != nil {
			return err
		}
	}

	if !equals(oldObj.ObjectMeta.Labels, newObj.ObjectMeta.Labels) ||
		oldObj.Status.PodIP != newObj.Status.PodIP {

		for _, ps := range ns.podSelectors.entries {
			oldMatch := ps.matchesPodSelector(oldObj.ObjectMeta.Labels)
			newMatch := ps.matchesPodSelector(newObj.ObjectMeta.Labels)
			if oldMatch == newMatch && oldObj.Status.PodIP == newObj.Status.PodIP {
				continue
			}
			if oldMatch {
				if err := ps.delEntry(uid(oldObj), oldObj.Status.PodIP); err != nil {
					return err
				}
			}
			if newMatch {
				if err := ps.addEntry(uid(newObj), newObj.Status.PodIP, podComment(newObj)); err != nil {
					return err
				}
			}

			if err := ns.addOrRemoveToDefaultAllowIPSet(ps, oldObj, newObj, oldMatch, newMatch, policyTypeIngress); err != nil {
				return err
			}
			if err := ns.addOrRemoveToDefaultAllowIPSet(ps, oldObj, newObj, oldMatch, newMatch, policyTypeEgress); err != nil {
				return err
			}
		}
		for _, ps := range ns.namespacedPodsSelectors.entries {
			oldMatch := ps.matchesNamespacedPodSelector(oldObj.ObjectMeta.Labels, ns.namespaceLabels)
			newMatch := ps.matchesNamespacedPodSelector(newObj.ObjectMeta.Labels, ns.namespaceLabels)
			if oldMatch == newMatch && oldObj.Status.PodIP == newObj.Status.PodIP {
				continue
			}
			if oldMatch {
				if err := ps.delEntry(uid(oldObj), oldObj.Status.PodIP); err != nil {
					return err
				}
			}
			if newMatch {
				if err := ps.addEntry(uid(newObj), newObj.Status.PodIP, podComment(newObj)); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (ns *ns) addOrRemoveToDefaultAllowIPSet(ps *selector, oldObj, newObj *coreapi.Pod, oldMatch, newMatch bool, policyType policyType) error {
	ipset := ns.defaultAllowIPSetName(policyType)
	if ns.podSelectors.targetSelectorExist(ps, policyType) {
		switch {
		case !oldMatch && newMatch:
			if err := ns.ips.DelEntry(uid(oldObj), ipset, oldObj.Status.PodIP); err != nil {
				return err
			}
		case oldMatch && !newMatch:
			if err := ns.addToDefaultAllowIfNoMatching(newObj, policyType); err != nil {
				return err
			}
		}
	}
	return nil
}

func (ns *ns) deletePod(obj *coreapi.Pod) error {
	delete(ns.pods, uid(obj))

	if !hasIP(obj) {
		return nil
	}

	if err := ns.ips.DelEntry(uid(obj), ns.ingressDefaultAllowIPSet, obj.Status.PodIP); err != nil {
		return err
	}
	if err := ns.ips.DelEntry(uid(obj), ns.egressDefaultAllowIPSet, obj.Status.PodIP); err != nil {
		return err
	}
	if err := ns.podSelectors.delFromMatchingPodSelector(uid(obj), obj.ObjectMeta.Labels, obj.Status.PodIP); err != nil {
		return err
	}
	if err := ns.namespacedPodsSelectors.delFromMatchingNamespacedPodSelector(uid(obj), obj.ObjectMeta.Labels, ns.namespaceLabels, obj.Status.PodIP); err != nil {
		return err
	}
	return nil
}

func (ns *ns) addNetworkPolicy(obj interface{}) error {
	// Analyse policy, determine which rules and ipsets are required

	uid, rules, nsSelectors, podSelectors, namespacedPodsSelectors, ipBlocks, err := ns.analyse(obj)
	if err != nil {
		return err
	}

	// Provision required resources in dependency order
	if err := ns.nsSelectors.provision(uid, nil, nsSelectors); err != nil {
		return err
	}
	if err := ns.podSelectors.provision(uid, nil, podSelectors); err != nil {
		return err
	}
	if err := ns.namespacedPodsSelectors.provision(uid, nil, namespacedPodsSelectors); err != nil {
		return err
	}
	if err := ns.ipBlocks.provision(uid, nil, ipBlocks); err != nil {
		return err
	}
	return ns.rules.provision(uid, nil, rules)
}

func (ns *ns) updateNetworkPolicy(oldObj, newObj interface{}) error {
	// Analyse the old and the new policy so we can determine differences
	oldUID, oldRules, oldNsSelectors, oldPodSelectors, oldNamespacedPodsSelectors, oldIPBlocks, err := ns.analyse(oldObj)
	if err != nil {
		return err
	}
	newUID, newRules, newNsSelectors, newPodSelectors, newNamespacedPodsSelectors, newIPBlocks, err := ns.analyse(newObj)
	if err != nil {
		return err
	}

	delete(ns.policies, oldUID)
	ns.policies[newUID] = newObj

	// Deprovision unused and provision newly required resources in dependency order
	if err := ns.rules.deprovision(oldUID, oldRules, newRules); err != nil {
		return err
	}
	if err := ns.nsSelectors.deprovision(oldUID, oldNsSelectors, newNsSelectors); err != nil {
		return err
	}
	if err := ns.podSelectors.deprovision(oldUID, oldPodSelectors, newPodSelectors); err != nil {
		return err
	}
	if err := ns.namespacedPodsSelectors.deprovision(oldUID, oldNamespacedPodsSelectors, newNamespacedPodsSelectors); err != nil {
		return err
	}
	if err := ns.ipBlocks.deprovision(oldUID, oldIPBlocks, newIPBlocks); err != nil {
		return err
	}
	if err := ns.nsSelectors.provision(oldUID, oldNsSelectors, newNsSelectors); err != nil {
		return err
	}
	if err := ns.podSelectors.provision(oldUID, oldPodSelectors, newPodSelectors); err != nil {
		return err
	}
	if err := ns.namespacedPodsSelectors.provision(oldUID, oldNamespacedPodsSelectors, newNamespacedPodsSelectors); err != nil {
		return err
	}
	if err := ns.ipBlocks.provision(oldUID, oldIPBlocks, newIPBlocks); err != nil {
		return err
	}
	return ns.rules.provision(oldUID, oldRules, newRules)
}

func (ns *ns) deleteNetworkPolicy(obj interface{}) error {
	// Analyse network policy to free resources
	uid, rules, nsSelectors, podSelectors, namespacedPodsSelectors, ipBlocks, err := ns.analyse(obj)
	if err != nil {
		return err
	}

	delete(ns.policies, uid)

	// Deprovision unused resources in dependency order
	if err := ns.rules.deprovision(uid, rules, nil); err != nil {
		return err
	}
	if err := ns.nsSelectors.deprovision(uid, nsSelectors, nil); err != nil {
		return err
	}
	if err := ns.podSelectors.deprovision(uid, podSelectors, nil); err != nil {
		return err
	}
	if err := ns.namespacedPodsSelectors.deprovision(uid, namespacedPodsSelectors, nil); err != nil {
		return err
	}
	if err := ns.ipBlocks.deprovision(uid, ipBlocks, nil); err != nil {
		return err
	}

	return nil
}

func (ns *ns) updateDefaultAllowIPSetEntry(oldObj, newObj *coreapi.Pod, ipsetName ipset.Name) error {
	// Instead of iterating over all selectors we check whether old pod IP
	// has been inserted into default-allow ipset to decide whether the IP
	// in the ipset has to be updated.
	if ns.ips.EntryExists(uid(oldObj), ipsetName, oldObj.Status.PodIP) {

		if err := ns.ips.DelEntry(uid(oldObj), ipsetName, oldObj.Status.PodIP); err != nil {
			return err
		}
		if err := ns.ips.AddEntry(uid(newObj), ipsetName, newObj.Status.PodIP, podComment(newObj)); err != nil {
			return err
		}
	}
	return nil
}

func bypassRules(namespace string, ingress, egress ipset.Name) map[string][][]string {
	return map[string][][]string{
		chains.DefaultChain: {
			{"-m", "set", "--match-set", string(ingress), "dst", "-j", "ACCEPT",
				"-m", "comment", "--comment", "DefaultAllow ingress isolation for namespace: " + namespace},
		},
		chains.EgressDefaultChain: {
			{"-m", "set", "--match-set", string(egress), "src", "-j", chains.EgressMarkChain,
				"-m", "comment", "--comment", "DefaultAllow egress isolation for namespace: " + namespace},
			{"-m", "set", "--match-set", string(egress), "src", "-j", "RETURN",
				"-m", "comment", "--comment", "DefaultAllow egress isolation for namespace: " + namespace},
		},
	}
}

func (ns *ns) ensureBypassRules() error {
	for chain, rules := range bypassRules(ns.name, ns.ingressDefaultAllowIPSet, ns.egressDefaultAllowIPSet) {
		for _, rule := range rules {
			common.Log.Debugf("adding rule for DefaultAllow in namespace: %s, chain: %s, %s", ns.name, chain, rule)
			if err := ns.ipt.Append(TableFilter, chain, rule...); err != nil {
				return err
			}
		}
	}

	return nil
}

func (ns *ns) deleteBypassRules() error {
	for chain, rules := range bypassRules(ns.name, ns.ingressDefaultAllowIPSet, ns.egressDefaultAllowIPSet) {
		for _, rule := range rules {
			common.Log.Debugf("removing rule for DefaultAllow in namespace: %s, chain: %s, %s", ns.name, chain, rule)
			if err := ns.ipt.Delete(TableFilter, chain, rule...); err != nil {
				return err
			}
		}
	}

	return nil
}

func (ns *ns) addNamespace(obj *coreapi.Namespace) error {
	ns.namespaceUID = ipset.UID(obj.ObjectMeta.UID)
	ns.namespaceLabels = obj.ObjectMeta.Labels

	// Insert a rule to bypass policies
	if err := ns.ensureBypassRules(); err != nil {
		return err
	}

	// Add namespace ipset to matching namespace selectors
	err := ns.nsSelectors.addToMatchingNamespaceSelector(nsuid(obj), obj.ObjectMeta.Labels, string(ns.allPods.ipsetName), namespaceComment(ns))
	return err
}

func (ns *ns) updateNamespace(oldObj, newObj *coreapi.Namespace) error {
	// Re-evaluate namespace selector membership if labels have changed
	if !equals(oldObj.ObjectMeta.Labels, newObj.ObjectMeta.Labels) {
		ns.namespaceLabels = newObj.ObjectMeta.Labels

		for _, selector := range ns.nsSelectors.entries {
			oldMatch := selector.matchesNamespaceSelector(oldObj.ObjectMeta.Labels)
			newMatch := selector.matchesNamespaceSelector(newObj.ObjectMeta.Labels)
			if oldMatch == newMatch {
				continue
			}
			if oldMatch {
				if err := selector.delEntry(ns.namespaceUID, string(ns.allPods.ipsetName)); err != nil {
					return err
				}
			}
			if newMatch {
				if err := selector.addEntry(ns.namespaceUID, string(ns.allPods.ipsetName), namespaceComment(ns)); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (ns *ns) deleteNamespace(obj *coreapi.Namespace) error {
	ns.namespaceUID = ""

	// Remove bypass rule
	if err := ns.deleteBypassRules(); err != nil {
		return err
	}

	// Remove namespace ipset from any matching namespace selectors
	err := ns.nsSelectors.delFromMatchingNamespaceSelector(nsuid(obj), obj.ObjectMeta.Labels, string(ns.allPods.ipsetName))
	if err != nil {
		return err
	}

	if err := ns.ips.Destroy(ns.ingressDefaultAllowIPSet); err != nil {
		return err
	}
	if err := ns.ips.Destroy(ns.egressDefaultAllowIPSet); err != nil {
		return err
	}

	return nil
}

func hasIP(pod *coreapi.Pod) bool {
	// Ensure pod isn't dead, has an IP address and isn't sharing the host network namespace
	return pod.Status.Phase != "Succeeded" && pod.Status.Phase != "Failed" &&
		len(pod.Status.PodIP) > 0 && !pod.Spec.HostNetwork
}

func equals(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for ak, av := range a {
		if b[ak] != av {
			return false
		}
	}
	return true
}

func namespaceComment(namespace *ns) string {
	return "namespace: " + namespace.name
}

func podComment(pod *coreapi.Pod) string {
	return fmt.Sprintf("namespace: %s, pod: %s", pod.ObjectMeta.Namespace, pod.ObjectMeta.Name)
}

func (ns *ns) defaultAllowIPSetName(pt policyType) ipset.Name {
	ipset := ns.ingressDefaultAllowIPSet
	if pt == policyTypeEgress {
		ipset = ns.egressDefaultAllowIPSet
	}
	return ipset
}

func (ns *ns) analyse(obj interface{}) (
	uid ipset.UID,
	rules map[string]*ruleSpec,
	nsSelectors, podSelectors, namespacedPodsSelectors map[string]*selectorSpec,
	ipBlocks map[string]*ipBlockSpec,
	err error) {

	switch p := obj.(type) {
	case *extnapi.NetworkPolicy:
		uid = ipset.UID(p.ObjectMeta.UID)
	case *networkingv1.NetworkPolicy:
		uid = ipset.UID(p.ObjectMeta.UID)
	default:
		err = errInvalidNetworkPolicyObjType
		return
	}
	ns.policies[uid] = obj

	// Analyse policy, determine which rules and ipsets are required
	rules, nsSelectors, podSelectors, namespacedPodsSelectors, ipBlocks, err = ns.analysePolicy(obj.(*networkingv1.NetworkPolicy))
	if err != nil {
		return
	}

	return
}
