package main

import (
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/npc"
	coreapi "k8s.io/api/core/v1"
	api "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"os/exec"
)

// replicated from kube-peer
const (
	// Prefix all our annotation keys with this string so they don't clash with anyone else's
	KubePeersPrefix = "kube-peers.weave.works/"
	// KubePeersAnnotationKey is the default annotation key
	KubePeersAnnotationKey = KubePeersPrefix + "peers"
)

type peerList struct {
	Peers []peerInfo
}

type peerInfo struct {
	PeerName string // Weave internal unique ID
	NodeName string // Kubernetes node name
	BridgeIP string // Weave bridge IP. May be empty.
}

func getBridgeIPs(clientset *kubernetes.Clientset) (ips map[string]string, err error) {
	cm, err := clientset.Core().ConfigMaps("kube-system").Get("weave-net", api.GetOptions{})
	if err != nil {
		return
	}

	if cm == nil {
		err = fmt.Errorf("no peer found")
		return
	}

	var record peerList
	if recordBytes, found := cm.Annotations[KubePeersAnnotationKey]; found {
		if err = json.Unmarshal([]byte(recordBytes), &record); err != nil {
			return nil, err
		}
	} else {
		err = fmt.Errorf("no peer found")
		return
	}

	ips = make(map[string]string)
	for _, peer := range record.Peers {
		if len(peer.BridgeIP) >= 0 {
			ips[peer.NodeName] = peer.BridgeIP
		} else {
			common.Log.Errorf("node %s didn't register is bridge IP", peer.NodeName)
		}
	}

	return
}

func doExec(args ...string) error {
	if output, err := exec.Command("ipset", args...).CombinedOutput(); err != nil {
		return errors.Wrapf(err, "ipset %v failed: %s", args, output)
	}
	return nil
}

func addBridgeIPSetEntry(entry string, comment string) error {
	common.Log.Printf("added entry %s to %s", entry, npc.BridgeIpset)
	if len(comment) > 0 {
		return doExec("add", npc.BridgeIpset, entry, "comment", comment)
	}
	return doExec("add", npc.BridgeIpset, entry)
}

func delBridgeIPSetEntry(entry string) error {
	common.Log.Printf("deleting entry %s from %s", entry, npc.BridgeIpset)
	return doExec("del", npc.BridgeIpset, entry)
}

type weaveDaemonController struct {
	cache.Controller
	store  cache.Store
	client *kubernetes.Clientset
}

func (c weaveDaemonController) updateBridgeIPs(deleted, added *coreapi.Pod) (err error) {
	bridgeIPs, err := getBridgeIPs(c.client)
	if err != nil {
		return
	}

	if added != nil {
		if len(added.Spec.NodeName) == 0 {
			common.Log.Warningf("no nodeName found for pod %s. It maybe not scheduled which phase is %s",
				added.Name, added.Status.Phase)
			return
		}

		if ip, found := bridgeIPs[added.Spec.NodeName]; found {
			common.Log.Infof("Add bridge ip of %s[%s] to bridge ipset", added.Spec.NodeName, ip)
			err = addBridgeIPSetEntry(ip, fmt.Sprintf("bridge IP of %s", added.Spec.NodeName))
			return
		}

		common.Log.Warningf("node %s didn't register is bridge IP", added.Spec.NodeName)
		return
	}

	if deleted != nil {
		if len(deleted.Spec.NodeName) == 0 {
			common.Log.Warningf("no nodeName found for pod %s. It maybe not scheduled which phase is %s",
				deleted.Name, deleted.Status.Phase)
			return
		}

		if ip, found := bridgeIPs[deleted.Spec.NodeName]; found {
			common.Log.Infof("Remove bridge ip of %s[%s] from bridge ipset", deleted.Spec.NodeName, ip)
			err = delBridgeIPSetEntry(ip)
			return
		}

		common.Log.Warningf("node %s didn't register is bridge IP", deleted.Spec.NodeName)
		return
	}

	return
}

func makeWeaveDaemonController(client *kubernetes.Clientset) cache.Controller {
	const (
		namespaceKubeSystem = "kube-system"
		daemonSetWeave      = "weave-net"
		labelSelector       = "name=" + daemonSetWeave
	)

	listFunc := func(options api.ListOptions) (runtime.Object, error) {
		options.LabelSelector = labelSelector
		return client.Core().RESTClient().Get().
			Namespace(namespaceKubeSystem).
			Resource("pods").
			VersionedParams(&options, api.ParameterCodec).
			Do().
			Get()
	}

	watchFunc := func(options api.ListOptions) (watch.Interface, error) {
		options.Watch = true
		options.LabelSelector = labelSelector
		return client.Core().RESTClient().Get().
			Namespace(namespaceKubeSystem).
			Resource("pods").
			VersionedParams(&options, api.ParameterCodec).
			Watch()
	}

	c := &weaveDaemonController{
		client: client,
	}

	store, controller := cache.NewInformer(&cache.ListWatch{ListFunc: listFunc, WatchFunc: watchFunc}, &coreapi.Pod{},
		0, cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				handleError(c.updateBridgeIPs(nil, obj.(*coreapi.Pod)))
			},
			DeleteFunc: func(obj interface{}) {
				handleError(c.updateBridgeIPs(obj.(*coreapi.Pod), nil))
			}})

	c.Controller = controller
	c.store = store
	return c
}
