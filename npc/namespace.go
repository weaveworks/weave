package npc

import (
	"encoding/json"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	coreapi "k8s.io/client-go/pkg/api/v1"
	extnapi "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	networkingv1 "k8s.io/client-go/pkg/apis/networking/v1"

	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/npc/ipset"
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

	uid     types.UID     // surrogate UID to own allPods selector and deny rules
	allPods *selectorSpec // hash:ip ipset of all pod IPs in this namespace

	nsSelectors  *selectorSet
	podSelectors *selectorSet
	rules        *ruleSet
}

func newNS(name, nodeName string, ipt iptables.Interface, ips ipset.Interface, nsSelectors *selectorSet) (*ns, error) {
	allPods, err := newSelectorSpec(&metav1.LabelSelector{}, name, ipset.HashIP)
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
		rules:       newRuleSet(ipt),
	}

	ns.podSelectors = newSelectorSet(ips, ns.onNewPodSelector)

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
				if err := selector.addEntry(pod.Status.PodIP, podComment(pod)); err != nil {
					return err
				}
			}
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

	if ns.checkLocalPod(obj) {
		ns.ips.AddEntry(LocalIpset, obj.Status.PodIP, podComment(obj))
	}
	return ns.podSelectors.addToMatching(obj.ObjectMeta.Labels, obj.Status.PodIP, podComment(obj))
}

func (ns *ns) updatePod(oldObj, newObj *coreapi.Pod) error {
	delete(ns.pods, oldObj.ObjectMeta.UID)
	ns.pods[newObj.ObjectMeta.UID] = newObj

	if !hasIP(oldObj) && !hasIP(newObj) {
		return nil
	}

	if hasIP(oldObj) && !hasIP(newObj) {
		if ns.checkLocalPod(oldObj) {
			ns.ips.DelEntry(LocalIpset, oldObj.Status.PodIP)
		}
		return ns.podSelectors.delFromMatching(oldObj.ObjectMeta.Labels, oldObj.Status.PodIP)
	}

	if !hasIP(oldObj) && hasIP(newObj) {
		if ns.checkLocalPod(newObj) {
			ns.ips.AddEntry(LocalIpset, newObj.Status.PodIP, podComment(newObj))
		}
		return ns.podSelectors.addToMatching(newObj.ObjectMeta.Labels, newObj.Status.PodIP, podComment(newObj))
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
				if err := ps.delEntry(oldObj.Status.PodIP); err != nil {
					return err
				}
			}
			if newMatch {
				if err := ps.addEntry(newObj.Status.PodIP, podComment(newObj)); err != nil {
					return err
				}
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

	if ns.checkLocalPod(obj) {
		ns.ips.DelEntry(LocalIpset, obj.Status.PodIP)
	}
	return ns.podSelectors.delFromMatching(obj.ObjectMeta.Labels, obj.Status.PodIP)
}

func (ns *ns) addNetworkPolicy(obj interface{}, legacy bool) error {
	// Analyse policy, determine which rules and ipsets are required

	uid, rules, nsSelectors, podSelectors, err := ns.analyse(obj, legacy)
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
	return ns.rules.provision(uid, nil, rules)
}

func (ns *ns) updateNetworkPolicy(oldObj, newObj interface{}, legacy bool) error {
	// Analyse the old and the new policy so we can determine differences
	oldUID, oldRules, oldNsSelectors, oldPodSelectors, err := ns.analyse(oldObj, legacy)
	if err != nil {
		return err
	}
	newUID, newRules, newNsSelectors, newPodSelectors, err := ns.analyse(newObj, legacy)
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
	if err := ns.nsSelectors.provision(oldUID, oldNsSelectors, newNsSelectors); err != nil {
		return err
	}
	if err := ns.podSelectors.provision(oldUID, oldPodSelectors, newPodSelectors); err != nil {
		return err
	}
	return ns.rules.provision(oldUID, oldRules, newRules)
}

func (ns *ns) deleteNetworkPolicy(obj interface{}, legacy bool) error {

	// Analyse network policy to free resources
	uid, rules, nsSelectors, podSelectors, err := ns.analyse(obj, legacy)
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
	return ns.podSelectors.deprovision(uid, podSelectors, nil)
}

func bypassRule(nsIpsetName ipset.Name, namespace string) []string {
	return []string{"-m", "set", "--match-set", string(nsIpsetName), "dst", "-j", "ACCEPT", "-m", "comment", "--comment", "DefaultAllow isolation for namespace: " + namespace}
}

func (ns *ns) ensureDenyDefault() error {
	common.Log.Debugf("ensuring rule for DefaultDeny in namespace: %s", ns.name)
	rule := newRuleSpecDeny(ns.allPods)

	return ns.rules.provision(ns.uid, nil, map[string]*ruleSpec{rule.key: rule})
}

func (ns *ns) delDenyDefault() error {
	common.Log.Debugf("removing DefaultDeny rule in namespace: %s", ns.name)
	rule := newRuleSpecDeny(ns.allPods)

	return ns.rules.deprovision(ns.uid, map[string]*ruleSpec{rule.key: rule}, nil)
}

func (ns *ns) addNamespace(obj *coreapi.Namespace, legacy bool) error {
	ns.namespace = obj

	// Insert a rule to deny all ingress traffic if namespace is DefaultDeny and NPC runs in legacy mode
	if isDefaultDeny(obj, legacy) {
		if err := ns.ensureDenyDefault(); err != nil {
			return err
		}
	}

	// Add namespace ipset to matching namespace selectors
	return ns.nsSelectors.addToMatching(obj.ObjectMeta.Labels, string(ns.allPods.ipsetName), namespaceComment(ns))
}

func (ns *ns) updateNamespace(oldObj, newObj *coreapi.Namespace, legacy bool) error {
	ns.namespace = newObj

	// Update deny all rule if ingress default has changed
	oldDefaultDeny := isDefaultDeny(oldObj, legacy)
	newDefaultDeny := isDefaultDeny(newObj, legacy)

	if oldDefaultDeny != newDefaultDeny {
		common.Log.Infof("namespace DefaultDeny changed from %t to %t", oldDefaultDeny, newDefaultDeny)
		if oldDefaultDeny {
			if err := ns.delDenyDefault(); err != nil {
				return err
			}
		}
		if newDefaultDeny {
			if err := ns.ensureDenyDefault(); err != nil {
				return err
			}
		}
	}

	// Re-evaluate namespace selector membership if labels have changed
	if !equals(oldObj.ObjectMeta.Labels, newObj.ObjectMeta.Labels) {
		for _, selector := range ns.nsSelectors.entries {
			oldMatch := selector.matches(oldObj.ObjectMeta.Labels)
			newMatch := selector.matches(newObj.ObjectMeta.Labels)
			if oldMatch == newMatch {
				continue
			}
			if oldMatch {
				if err := selector.delEntry(string(ns.allPods.ipsetName)); err != nil {
					return err
				}
			}
			if newMatch {
				if err := selector.addEntry(string(ns.allPods.ipsetName), namespaceComment(ns)); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (ns *ns) deleteNamespace(obj *coreapi.Namespace, legacy bool) error {
	ns.namespace = nil

	// Remove bypass rule
	if isDefaultDeny(obj, legacy) {
		if err := ns.delDenyDefault(); err != nil {
			return err
		}
	}

	// Remove namespace ipset from any matching namespace selectors
	return ns.nsSelectors.delFromMatching(obj.ObjectMeta.Labels, string(ns.allPods.ipsetName))
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

func isDefaultDeny(namespace *coreapi.Namespace, legacy bool) bool {
	if !legacy {
		common.Log.Warn("DefaultDeny annotation is supported only in legacy mode (--use-legacy-netpol)")
		return false
	}

	nnpJSON, found := namespace.ObjectMeta.Annotations["net.beta.kubernetes.io/network-policy"]
	if !found {
		return false
	}

	var nnp NamespaceNetworkPolicy
	if err := json.Unmarshal([]byte(nnpJSON), &nnp); err != nil {
		common.Log.Warn("Ignoring network policy annotation: unmarshal failed:", err)
		// If we can't understand the annotation, behave as if it isn't present
		return false
	}

	return nnp.Ingress != nil &&
		nnp.Ingress.Isolation != nil &&
		*(nnp.Ingress.Isolation) == DefaultDeny
}

func namespaceComment(namespace *ns) string {
	return "namespace: " + namespace.name
}

func podComment(pod *coreapi.Pod) string {
	return fmt.Sprintf("namespace: %s, pod: %s", pod.ObjectMeta.Namespace, pod.ObjectMeta.Name)
}

func (ns *ns) analyse(obj interface{}, legacy bool) (
	uid types.UID,
	rules map[string]*ruleSpec,
	nsSelectors, podSelectors map[string]*selectorSpec,
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
	if legacy {
		rules, nsSelectors, podSelectors, err = ns.analysePolicyLegacy(obj.(*extnapi.NetworkPolicy))
		if err != nil {
			return
		}
	} else {
		rules, nsSelectors, podSelectors, err = ns.analysePolicy(obj.(*networkingv1.NetworkPolicy))
		if err != nil {
			return
		}
	}

	return
}
