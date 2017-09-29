package npc

import (
	"sync"

	"github.com/pkg/errors"
	coreapi "k8s.io/client-go/pkg/api/v1"
	extnapi "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	networkingv1 "k8s.io/client-go/pkg/apis/networking/v1"

	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/npc/ipset"
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

	legacy bool // denotes whether to use legacy network policies (k8s pre-1.7)
}

func New(nodeName string, legacy bool, ipt iptables.Interface, ips ipset.Interface) NetworkPolicyController {
	c := &controller{
		nodeName: nodeName,
		legacy:   legacy,
		ipt:      ipt,
		ips:      ips,
		nss:      make(map[string]*ns)}

	c.nsSelectors = newSelectorSet(ips, c.onNewNsSelector)

	return c
}

func (npc *controller) onNewNsSelector(selector *selector) error {
	for _, ns := range npc.nss {
		if ns.namespace != nil {
			if selector.matches(ns.namespace.ObjectMeta.Labels) {
				if err := selector.addEntry(string(ns.allPods.ipsetName), namespaceComment(ns)); err != nil {
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
		return errors.Wrap(ns.addNetworkPolicy(obj, npc.legacy), "add network policy")
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
		return errors.Wrap(ns.updateNetworkPolicy(oldObj, newObj, npc.legacy), "update network policy")
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
		return errors.Wrap(ns.deleteNetworkPolicy(obj, npc.legacy), "delete network policy")
	})
}

func (npc *controller) AddNamespace(obj *coreapi.Namespace) error {
	npc.Lock()
	defer npc.Unlock()

	common.Log.Infof("EVENT AddNamespace %s", js(obj))
	return npc.withNS(obj.ObjectMeta.Name, func(ns *ns) error {
		return errors.Wrap(ns.addNamespace(obj, npc.legacy), "add namespace")
	})
}

func (npc *controller) UpdateNamespace(oldObj, newObj *coreapi.Namespace) error {
	npc.Lock()
	defer npc.Unlock()

	common.Log.Infof("EVENT UpdateNamespace %s %s", js(oldObj), js(newObj))
	return npc.withNS(oldObj.ObjectMeta.Name, func(ns *ns) error {
		return errors.Wrap(ns.updateNamespace(oldObj, newObj, npc.legacy), "update namespace")
	})
}

func (npc *controller) DeleteNamespace(obj *coreapi.Namespace) error {
	npc.Lock()
	defer npc.Unlock()

	common.Log.Infof("EVENT DeleteNamespace %s", js(obj))
	return npc.withNS(obj.ObjectMeta.Name, func(ns *ns) error {
		return errors.Wrap(ns.deleteNamespace(obj, npc.legacy), "delete namespace")
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
