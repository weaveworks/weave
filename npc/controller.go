package npc

import (
	"context"
	"fmt"
	"sync"

	"github.com/pkg/errors"
	coreapi "k8s.io/api/core/v1"
	extnapi "k8s.io/api/extensions/v1beta1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/rajch/weave/common"
	"github.com/rajch/weave/common/chains"
	"github.com/rajch/weave/net/ipset"
	"github.com/rajch/weave/npc/iptables"
)

type NetworkPolicyController interface {
	AddNamespace(ctx context.Context, ns *coreapi.Namespace) error
	UpdateNamespace(ctx context.Context, oldObj, newObj *coreapi.Namespace) error
	DeleteNamespace(ctx context.Context, ns *coreapi.Namespace) error

	AddPod(ctx context.Context, obj *coreapi.Pod) error
	UpdatePod(ctx context.Context, oldObj, newObj *coreapi.Pod) error
	DeletePod(ctx context.Context, obj *coreapi.Pod) error

	AddNetworkPolicy(ctx context.Context, obj interface{}) error
	UpdateNetworkPolicy(ctx context.Context, oldObj, newObj interface{}) error
	DeleteNetworkPolicy(ctx context.Context, obj interface{}) error
}

type controller struct {
	sync.Mutex

	nodeName string // my node name

	ipt                    iptables.Interface
	ips                    ipset.Interface
	clientset              kubernetes.Interface
	nss                    map[string]*ns // ns name -> ns struct
	nsSelectors            *selectorSet   // selector string -> nsSelector
	namespacedPodSelectors *selectorSet
	defaultEgressDrop      bool // flag to track if base iptable rule to drop egress traffic is added or not
}

func New(nodeName string, ipt iptables.Interface, ips ipset.Interface, clientset kubernetes.Interface) NetworkPolicyController {
	c := &controller{
		nodeName:  nodeName,
		ipt:       ipt,
		ips:       ips,
		clientset: clientset,
		nss:       make(map[string]*ns)}

	doNothing := func(*selector, policyType) error { return nil }
	c.nsSelectors = newSelectorSet(ips, c.onNewNsSelector, doNothing, doNothing)
	c.namespacedPodSelectors = newSelectorSet(ips, c.onNewNamespacePodsSelector, doNothing, doNothing)
	return c
}

func (npc *controller) onNewNsSelector(selector *selector) error {
	for _, ns := range npc.nss {
		if ns.namespaceUID != "" {
			if selector.matchesNamespaceSelector(ns.namespaceLabels) {
				if err := selector.addEntry(ns.namespaceUID, string(ns.allPods.ipsetName), namespaceComment(ns)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (npc *controller) onNewNamespacePodsSelector(selector *selector) error {
	for _, ns := range npc.nss {
		if ns.namespaceUID != "" && len(ns.pods) > 0 {
			for _, pod := range ns.pods {
				if hasIP(pod) {
					if selector.matchesNamespacedPodSelector(pod.ObjectMeta.Labels, ns.namespaceLabels) {
						if err := selector.addEntry(uid(pod), pod.Status.PodIP, podComment(pod)); err != nil {
							return err
						}

					}
				}
			}
		}
	}
	return nil
}

func (npc *controller) withNS(ctx context.Context, name string, f func(ns *ns) error) error {
	ns, found := npc.nss[name]
	if !found {
		namespace, err := npc.clientset.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		newNs, err := newNS(name, npc.nodeName, npc.ipt, npc.ips, npc.nsSelectors, npc.namespacedPodSelectors, namespace)
		if err != nil {
			return err
		}
		npc.nss[name] = newNs
		ns = newNs
	}
	if err := f(ns); err != nil {
		return err
	}
	if ns.empty() {
		if err := ns.destroy(); err != nil {
			return err
		}
		delete(npc.nss, name)
	}
	return nil
}

func (npc *controller) AddPod(ctx context.Context, obj *coreapi.Pod) error {
	npc.Lock()
	defer npc.Unlock()

	common.Log.Debugf("EVENT AddPod %s", js(obj))
	return npc.withNS(ctx, obj.ObjectMeta.Namespace, func(ns *ns) error {
		return errors.Wrap(ns.addPod(obj), "add pod")
	})
}

func (npc *controller) UpdatePod(ctx context.Context, oldObj, newObj *coreapi.Pod) error {
	npc.Lock()
	defer npc.Unlock()

	common.Log.Debugf("EVENT UpdatePod %s %s", js(oldObj), js(newObj))
	return npc.withNS(ctx, oldObj.ObjectMeta.Namespace, func(ns *ns) error {
		return errors.Wrap(ns.updatePod(oldObj, newObj), "update pod")
	})
}

func (npc *controller) DeletePod(ctx context.Context, obj *coreapi.Pod) error {
	npc.Lock()
	defer npc.Unlock()

	common.Log.Debugf("EVENT DeletePod %s", js(obj))
	return npc.withNS(ctx, obj.ObjectMeta.Namespace, func(ns *ns) error {
		return errors.Wrap(ns.deletePod(obj), "delete pod")
	})
}

func (npc *controller) AddNetworkPolicy(ctx context.Context, obj interface{}) error {
	npc.Lock()
	defer npc.Unlock()

	// lazily add default rule to drop egress traffic only when network policies are applied
	if !npc.defaultEgressDrop {
		egressNetworkPolicy, err := isEgressNetworkPolicy(obj)
		if err != nil {
			return err
		}
		if egressNetworkPolicy {
			if err := npc.ipt.Append(TableFilter, chains.EgressChain,
				"-m", "state", "--state", "NEW", "-m", "mark", "!", "--mark", EgressMark, "-j", "NFLOG", "--nflog-group", "86"); err != nil {
				return fmt.Errorf("Failed to add iptable egress log rule: %w", err)
			}
			if err := npc.ipt.Append(TableFilter, chains.EgressChain,
				"-m", "mark", "!", "--mark", EgressMark, "-j", "DROP"); err != nil {
				return fmt.Errorf("Failed to add iptable rule to drop egress traffic from the pods by default due to %s", err.Error())
			}
			npc.defaultEgressDrop = true
		}
	}

	nsName, err := nsName(obj)
	if err != nil {
		return err
	}
	common.Log.Infof("EVENT AddNetworkPolicy %s", js(obj))
	return npc.withNS(ctx, nsName, func(ns *ns) error {
		return errors.Wrap(ns.addNetworkPolicy(obj), "add network policy")
	})
}

func (npc *controller) UpdateNetworkPolicy(ctx context.Context, oldObj, newObj interface{}) error {
	npc.Lock()
	defer npc.Unlock()

	nsName, err := nsName(oldObj)
	if err != nil {
		return err
	}

	common.Log.Infof("EVENT UpdateNetworkPolicy %s %s", js(oldObj), js(newObj))
	return npc.withNS(ctx, nsName, func(ns *ns) error {
		return errors.Wrap(ns.updateNetworkPolicy(oldObj, newObj), "update network policy")
	})
}

func (npc *controller) DeleteNetworkPolicy(ctx context.Context, obj interface{}) error {
	npc.Lock()
	defer npc.Unlock()

	nsName, err := nsName(obj)
	if err != nil {
		return err
	}

	common.Log.Infof("EVENT DeleteNetworkPolicy %s", js(obj))
	return npc.withNS(ctx, nsName, func(ns *ns) error {
		return errors.Wrap(ns.deleteNetworkPolicy(obj), "delete network policy")
	})
}

func (npc *controller) AddNamespace(ctx context.Context, obj *coreapi.Namespace) error {
	npc.Lock()
	defer npc.Unlock()

	common.Log.Infof("EVENT AddNamespace %s", js(obj))
	return npc.withNS(ctx, obj.ObjectMeta.Name, func(ns *ns) error {
		return errors.Wrap(ns.addNamespace(obj), "add namespace")
	})
}

func (npc *controller) UpdateNamespace(ctx context.Context, oldObj, newObj *coreapi.Namespace) error {
	npc.Lock()
	defer npc.Unlock()

	common.Log.Infof("EVENT UpdateNamespace %s %s", js(oldObj), js(newObj))
	return npc.withNS(ctx, oldObj.ObjectMeta.Name, func(ns *ns) error {
		return errors.Wrap(ns.updateNamespace(oldObj, newObj), "update namespace")
	})
}

func (npc *controller) DeleteNamespace(ctx context.Context, obj *coreapi.Namespace) error {
	npc.Lock()
	defer npc.Unlock()

	common.Log.Infof("EVENT DeleteNamespace %s", js(obj))
	return npc.withNS(ctx, obj.ObjectMeta.Name, func(ns *ns) error {
		return errors.Wrap(ns.deleteNamespace(obj), "delete namespace")
	})
}

func nsName(obj interface{}) (string, error) {
	switch obj := obj.(type) {
	case *networkingv1.NetworkPolicy:
		return obj.ObjectMeta.Namespace, nil
	case *extnapi.NetworkPolicy:
		return obj.ObjectMeta.Namespace, nil
	}

	return "", errInvalidNetworkPolicyObjType
}

func isEgressNetworkPolicy(obj interface{}) (bool, error) {
	if policy, ok := obj.(*networkingv1.NetworkPolicy); ok {
		if len(policy.Spec.PolicyTypes) > 0 {
			for _, policyType := range policy.Spec.PolicyTypes {
				if policyType == networkingv1.PolicyTypeEgress {
					return true, nil
				}
			}
		}
		if policy.Spec.Egress != nil {
			return true, nil
		}
		return false, nil
	}
	return false, errInvalidNetworkPolicyObjType
}

func uid(pod *coreapi.Pod) ipset.UID {
	return ipset.UID(pod.UID)
}

func nsuid(ns *coreapi.Namespace) ipset.UID {
	return ipset.UID(ns.UID)
}
