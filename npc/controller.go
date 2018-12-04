package npc

import (
	"sync"

	"github.com/pkg/errors"
	coreapi "k8s.io/api/core/v1"
	extnapi "k8s.io/api/extensions/v1beta1"
	networkingv1 "k8s.io/api/networking/v1"

	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/net/ipset"
	"github.com/weaveworks/weave/npc/iptables"
)

type NetworkPolicyController interface {
	AddNamespace(ns *coreapi.Namespace) error
	UpdateNamespace(oldObj, newObj *coreapi.Namespace) error
	DeleteNamespace(ns *coreapi.Namespace) error

	AddPod(obj *coreapi.Pod) error
	UpdatePod(oldObj, newObj *coreapi.Pod) error
	DeletePod(obj *coreapi.Pod) error

	AddNetworkPolicy(obj interface{}) error
	UpdateNetworkPolicy(oldObj, newObj interface{}) error
	DeleteNetworkPolicy(obj interface{}) error
}

type controller struct {
	sync.Mutex

	nodeName string // my node name

	ipt iptables.Interface
	ips ipset.Interface

	nss         map[string]*ns // ns name -> ns struct
	nsSelectors *selectorSet   // selector string -> nsSelector
}

func New(nodeName string, ipt iptables.Interface, ips ipset.Interface) NetworkPolicyController {
	c := &controller{
		nodeName: nodeName,
		ipt:      ipt,
		ips:      ips,
		nss:      make(map[string]*ns)}

	doNothing := func(*selector, policyType) error { return nil }
	c.nsSelectors = newSelectorSet(ips, c.onNewNsSelector, doNothing, doNothing)

	return c
}

func (npc *controller) onNewNsSelector(selector *selector) error {
	for _, ns := range npc.nss {
		if ns.namespace != nil {
			if selector.matches(ns.namespace.ObjectMeta.Labels) {
				if err := selector.addEntry(ns.namespace.ObjectMeta.UID, string(ns.allPods.ipsetName), namespaceComment(ns)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (npc *controller) withNS(name string, f func(ns *ns) error) error {
	ns, found := npc.nss[name]
	if !found {
		newNs, err := newNS(name, npc.nodeName, npc.ipt, npc.ips, npc.nsSelectors)
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

func (npc *controller) AddPod(obj *coreapi.Pod) error {
	npc.Lock()
	defer npc.Unlock()

	common.Log.Debugf("EVENT AddPod %s", js(obj))
	return npc.withNS(obj.ObjectMeta.Namespace, func(ns *ns) error {
		return errors.Wrap(ns.addPod(obj), "add pod")
	})
}

func (npc *controller) UpdatePod(oldObj, newObj *coreapi.Pod) error {
	npc.Lock()
	defer npc.Unlock()

	common.Log.Debugf("EVENT UpdatePod %s %s", js(oldObj), js(newObj))
	return npc.withNS(oldObj.ObjectMeta.Namespace, func(ns *ns) error {
		return errors.Wrap(ns.updatePod(oldObj, newObj), "update pod")
	})
}

func (npc *controller) DeletePod(obj *coreapi.Pod) error {
	npc.Lock()
	defer npc.Unlock()

	common.Log.Debugf("EVENT DeletePod %s", js(obj))
	return npc.withNS(obj.ObjectMeta.Namespace, func(ns *ns) error {
		return errors.Wrap(ns.deletePod(obj), "delete pod")
	})
}

func (npc *controller) AddNetworkPolicy(obj interface{}) error {
	npc.Lock()
	defer npc.Unlock()

	nsName, err := nsName(obj)
	if err != nil {
		return err
	}

	common.Log.Infof("EVENT AddNetworkPolicy %s", js(obj))
	return npc.withNS(nsName, func(ns *ns) error {
		return errors.Wrap(ns.addNetworkPolicy(obj), "add network policy")
	})
}

func (npc *controller) UpdateNetworkPolicy(oldObj, newObj interface{}) error {
	npc.Lock()
	defer npc.Unlock()

	nsName, err := nsName(oldObj)
	if err != nil {
		return err
	}

	common.Log.Infof("EVENT UpdateNetworkPolicy %s %s", js(oldObj), js(newObj))
	return npc.withNS(nsName, func(ns *ns) error {
		return errors.Wrap(ns.updateNetworkPolicy(oldObj, newObj), "update network policy")
	})
}

func (npc *controller) DeleteNetworkPolicy(obj interface{}) error {
	npc.Lock()
	defer npc.Unlock()

	nsName, err := nsName(obj)
	if err != nil {
		return err
	}

	common.Log.Infof("EVENT DeleteNetworkPolicy %s", js(obj))
	return npc.withNS(nsName, func(ns *ns) error {
		return errors.Wrap(ns.deleteNetworkPolicy(obj), "delete network policy")
	})
}

func (npc *controller) AddNamespace(obj *coreapi.Namespace) error {
	npc.Lock()
	defer npc.Unlock()

	common.Log.Infof("EVENT AddNamespace %s", js(obj))
	return npc.withNS(obj.ObjectMeta.Name, func(ns *ns) error {
		return errors.Wrap(ns.addNamespace(obj), "add namespace")
	})
}

func (npc *controller) UpdateNamespace(oldObj, newObj *coreapi.Namespace) error {
	npc.Lock()
	defer npc.Unlock()

	common.Log.Infof("EVENT UpdateNamespace %s %s", js(oldObj), js(newObj))
	return npc.withNS(oldObj.ObjectMeta.Name, func(ns *ns) error {
		return errors.Wrap(ns.updateNamespace(oldObj, newObj), "update namespace")
	})
}

func (npc *controller) DeleteNamespace(obj *coreapi.Namespace) error {
	npc.Lock()
	defer npc.Unlock()

	common.Log.Infof("EVENT DeleteNamespace %s", js(obj))
	return npc.withNS(obj.ObjectMeta.Name, func(ns *ns) error {
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
