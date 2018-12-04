package npc

import (
	"errors"
	"fmt"

	coreapi "k8s.io/api/core/v1"
	extnapi "k8s.io/api/extensions/v1beta1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"

	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/net/ipset"
	"github.com/weaveworks/weave/npc/iptables"
)

var errInvalidNetworkPolicyObjType = errors.New("invalid NetworkPolicy object type")

type ns struct {
	ipt iptables.Interface // interface to iptables
	ips ipset.Interface    // interface to ipset

	name      string                     // k8s Namespace name
	nodeName  string                     // my node name
	namespace *coreapi.Namespace         // k8s Namespace object
	pods      map[types.UID]*coreapi.Pod // k8s Pod objects by UID
	policies  map[types.UID]interface{}  // k8s NetworkPolicy objects by UID

	uid     types.UID     // surrogate UID to own allPods selector
	allPods *selectorSpec // hash:ip ipset of all pod IPs in this namespace

	// stores IP addrs of pods which are not selected by any target podSelector of
	// any netpol; used as a target in the WEAVE-NPC-{INGRESS,EGRESS}-DEFAULT
	// iptables chains.
	ingressDefaultAllowIPSet ipset.Name
	egressDefaultAllowIPSet  ipset.Name

	nsSelectors  *selectorSet
	podSelectors *selectorSet
	ipBlocks     *ipBlockSet
	rules        *ruleSet
}

func newNS(name, nodeName string, ipt iptables.Interface, ips ipset.Interface, nsSelectors *selectorSet) (*ns, error) {
	allPods, err := newSelectorSpec(&metav1.LabelSelector{}, nil, name, ipset.HashIP)
	if err != nil {
		return nil, err
	}

	ns := &ns{
		ipt:         ipt,
		ips:         ips,
		name:        name,
		nodeName:    nodeName,
		pods:        make(map[types.UID]*coreapi.Pod),
		policies:    make(map[types.UID]interface{}),
		uid:         uuid.NewUUID(),
		allPods:     allPods,
		nsSelectors: nsSelectors,
		ipBlocks:    newIPBlockSet(ips),
		rules:       newRuleSet(ipt),
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
	return len(ns.pods) == 0 && len(ns.policies) == 0 && ns.namespace == nil
}

func (ns *ns) destroy() error {
	return ns.podSelectors.deprovision(ns.uid, map[string]*selectorSpec{ns.allPods.key: ns.allPods}, nil)
}

func (ns *ns) onNewPodSelector(selector *selector) error {
	for _, pod := range ns.pods {
		if hasIP(pod) {
			if selector.matches(pod.ObjectMeta.Labels) {
				if err := selector.addEntry(pod.ObjectMeta.UID, pod.Status.PodIP, podComment(pod)); err != nil {
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
			if selector.matches(pod.ObjectMeta.Labels) {
				if err := ns.ips.DelEntry(pod.ObjectMeta.UID, ipset, pod.Status.PodIP); err != nil {
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
			if selector.matches(pod.ObjectMeta.Labels) {
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
		if ns.podSelectors.targetSelectorExist(s, policyType) && s.matches(pod.ObjectMeta.Labels) {
			found = true
			break
		}
	}
	if !found {
		ipset := ns.defaultAllowIPSetName(policyType)
		if err := ns.ips.AddEntry(pod.ObjectMeta.UID, ipset, pod.Status.PodIP, podComment(pod)); err != nil {
			return err
		}
	}
	return nil
}

func (ns *ns) checkLocalPod(obj *coreapi.Pod) bool {
	if obj.Spec.NodeName != ns.nodeName {
		return false
	}
	return true
}

func (ns *ns) addPod(obj *coreapi.Pod) error {
	ns.pods[obj.ObjectMeta.UID] = obj

	if !hasIP(obj) {
		return nil
	}

	foundIngress, foundEgress, err := ns.podSelectors.addToMatching(obj.ObjectMeta.UID, obj.ObjectMeta.Labels, obj.Status.PodIP, podComment(obj))
	if err != nil {
		return err
	}
	// If there are no matching target selectors, add the pod to default-allow
	if !foundIngress {
		if err := ns.ips.AddEntry(obj.ObjectMeta.UID, ns.ingressDefaultAllowIPSet, obj.Status.PodIP, podComment(obj)); err != nil {
			return err
		}
	}
	if !foundEgress {
		if err := ns.ips.AddEntry(obj.ObjectMeta.UID, ns.egressDefaultAllowIPSet, obj.Status.PodIP, podComment(obj)); err != nil {
			return err
		}
	}

	return nil
}

func (ns *ns) updatePod(oldObj, newObj *coreapi.Pod) error {
	delete(ns.pods, oldObj.ObjectMeta.UID)
	ns.pods[newObj.ObjectMeta.UID] = newObj

	if !hasIP(oldObj) && !hasIP(newObj) {
		return nil
	}

	if hasIP(oldObj) && !hasIP(newObj) {
		if err := ns.ips.DelEntry(oldObj.ObjectMeta.UID, ns.ingressDefaultAllowIPSet, oldObj.Status.PodIP); err != nil {
			return err
		}
		if err := ns.ips.DelEntry(oldObj.ObjectMeta.UID, ns.egressDefaultAllowIPSet, oldObj.Status.PodIP); err != nil {
			return err
		}

		return ns.podSelectors.delFromMatching(oldObj.ObjectMeta.UID, oldObj.ObjectMeta.Labels, oldObj.Status.PodIP)
	}

	if !hasIP(oldObj) && hasIP(newObj) {
		foundIngress, foundEgress, err := ns.podSelectors.addToMatching(newObj.ObjectMeta.UID, newObj.ObjectMeta.Labels, newObj.Status.PodIP, podComment(newObj))
		if err != nil {
			return err
		}

		if !foundIngress {
			if err := ns.ips.AddEntry(newObj.ObjectMeta.UID, ns.ingressDefaultAllowIPSet, newObj.Status.PodIP, podComment(newObj)); err != nil {
				return err
			}
		}
		if !foundEgress {
			if err := ns.ips.AddEntry(newObj.ObjectMeta.UID, ns.egressDefaultAllowIPSet, newObj.Status.PodIP, podComment(newObj)); err != nil {
				return err
			}
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
			oldMatch := ps.matches(oldObj.ObjectMeta.Labels)
			newMatch := ps.matches(newObj.ObjectMeta.Labels)
			if oldMatch == newMatch && oldObj.Status.PodIP == newObj.Status.PodIP {
				continue
			}
			if oldMatch {
				if err := ps.delEntry(oldObj.ObjectMeta.UID, oldObj.Status.PodIP); err != nil {
					return err
				}
			}
			if newMatch {
				if err := ps.addEntry(newObj.ObjectMeta.UID, newObj.Status.PodIP, podComment(newObj)); err != nil {
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
	}

	return nil
}

func (ns *ns) addOrRemoveToDefaultAllowIPSet(ps *selector, oldObj, newObj *coreapi.Pod, oldMatch, newMatch bool, policyType policyType) error {
	ipset := ns.defaultAllowIPSetName(policyType)
	if ns.podSelectors.targetSelectorExist(ps, policyType) {
		switch {
		case !oldMatch && newMatch:
			if err := ns.ips.DelEntry(oldObj.ObjectMeta.UID, ipset, oldObj.Status.PodIP); err != nil {
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
	delete(ns.pods, obj.ObjectMeta.UID)

	if !hasIP(obj) {
		return nil
	}

	if err := ns.ips.DelEntry(obj.ObjectMeta.UID, ns.ingressDefaultAllowIPSet, obj.Status.PodIP); err != nil {
		return err
	}
	if err := ns.ips.DelEntry(obj.ObjectMeta.UID, ns.egressDefaultAllowIPSet, obj.Status.PodIP); err != nil {
		return err
	}

	return ns.podSelectors.delFromMatching(obj.ObjectMeta.UID, obj.ObjectMeta.Labels, obj.Status.PodIP)
}

func (ns *ns) addNetworkPolicy(obj interface{}) error {
	// Analyse policy, determine which rules and ipsets are required

	uid, rules, nsSelectors, podSelectors, ipBlocks, err := ns.analyse(obj)
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
	if err := ns.ipBlocks.provision(uid, nil, ipBlocks); err != nil {
		return err
	}
	return ns.rules.provision(uid, nil, rules)
}

func (ns *ns) updateNetworkPolicy(oldObj, newObj interface{}) error {
	// Analyse the old and the new policy so we can determine differences
	oldUID, oldRules, oldNsSelectors, oldPodSelectors, oldIPBlocks, err := ns.analyse(oldObj)
	if err != nil {
		return err
	}
	newUID, newRules, newNsSelectors, newPodSelectors, newIPBlocks, err := ns.analyse(newObj)
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
	if err := ns.ipBlocks.deprovision(oldUID, oldIPBlocks, newIPBlocks); err != nil {
		return err
	}
	if err := ns.nsSelectors.provision(oldUID, oldNsSelectors, newNsSelectors); err != nil {
		return err
	}
	if err := ns.podSelectors.provision(oldUID, oldPodSelectors, newPodSelectors); err != nil {
		return err
	}
	if err := ns.ipBlocks.provision(oldUID, oldIPBlocks, newIPBlocks); err != nil {
		return err
	}
	return ns.rules.provision(oldUID, oldRules, newRules)
}

func (ns *ns) deleteNetworkPolicy(obj interface{}) error {
	// Analyse network policy to free resources
	uid, rules, nsSelectors, podSelectors, ipBlocks, err := ns.analyse(obj)
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
	if err := ns.ipBlocks.deprovision(uid, ipBlocks, nil); err != nil {
		return err
	}

	return nil
}

func (ns *ns) updateDefaultAllowIPSetEntry(oldObj, newObj *coreapi.Pod, ipsetName ipset.Name) error {
	// Instead of iterating over all selectors we check whether old pod IP
	// has been inserted into default-allow ipset to decide whether the IP
	// in the ipset has to be updated.
	if ns.ips.Exist(oldObj.ObjectMeta.UID, ipsetName, oldObj.Status.PodIP) {

		if err := ns.ips.DelEntry(oldObj.ObjectMeta.UID, ipsetName, oldObj.Status.PodIP); err != nil {
			return err
		}
		if err := ns.ips.AddEntry(newObj.ObjectMeta.UID, ipsetName, newObj.Status.PodIP, podComment(newObj)); err != nil {
			return err
		}
	}
	return nil
}

func bypassRules(namespace string, ingress, egress ipset.Name) map[string][][]string {
	return map[string][][]string{
		DefaultChain: {
			{"-m", "set", "--match-set", string(ingress), "dst", "-j", "ACCEPT",
				"-m", "comment", "--comment", "DefaultAllow ingress isolation for namespace: " + namespace},
		},
		EgressDefaultChain: {
			{"-m", "set", "--match-set", string(egress), "src", "-j", EgressMarkChain,
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
	ns.namespace = obj

	// Insert a rule to bypass policies
	if err := ns.ensureBypassRules(); err != nil {
		return err
	}

	// Add namespace ipset to matching namespace selectors
	_, _, err := ns.nsSelectors.addToMatching(obj.ObjectMeta.UID, obj.ObjectMeta.Labels, string(ns.allPods.ipsetName), namespaceComment(ns))
	return err
}

func (ns *ns) updateNamespace(oldObj, newObj *coreapi.Namespace) error {
	ns.namespace = newObj

	// Re-evaluate namespace selector membership if labels have changed
	if !equals(oldObj.ObjectMeta.Labels, newObj.ObjectMeta.Labels) {
		for _, selector := range ns.nsSelectors.entries {
			oldMatch := selector.matches(oldObj.ObjectMeta.Labels)
			newMatch := selector.matches(newObj.ObjectMeta.Labels)
			if oldMatch == newMatch {
				continue
			}
			if oldMatch {
				if err := selector.delEntry(ns.namespace.ObjectMeta.UID, string(ns.allPods.ipsetName)); err != nil {
					return err
				}
			}
			if newMatch {
				if err := selector.addEntry(ns.namespace.ObjectMeta.UID, string(ns.allPods.ipsetName), namespaceComment(ns)); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (ns *ns) deleteNamespace(obj *coreapi.Namespace) error {
	ns.namespace = nil

	// Remove bypass rule
	if err := ns.deleteBypassRules(); err != nil {
		return err
	}

	// Remove namespace ipset from any matching namespace selectors
	err := ns.nsSelectors.delFromMatching(obj.ObjectMeta.UID, obj.ObjectMeta.Labels, string(ns.allPods.ipsetName))
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
	uid types.UID,
	rules map[string]*ruleSpec,
	nsSelectors, podSelectors map[string]*selectorSpec,
	ipBlocks map[string]*ipBlockSpec,
	err error) {

	switch p := obj.(type) {
	case *extnapi.NetworkPolicy:
		uid = p.ObjectMeta.UID
	case *networkingv1.NetworkPolicy:
		uid = p.ObjectMeta.UID
	default:
		err = errInvalidNetworkPolicyObjType
		return
	}
	ns.policies[uid] = obj

	// Analyse policy, determine which rules and ipsets are required
	rules, nsSelectors, podSelectors, ipBlocks, err = ns.analysePolicy(obj.(*networkingv1.NetworkPolicy))
	if err != nil {
		return
	}

	return
}
