/*
Package main deals with weave-net peers on the cluster.

This involves peer management, such as getting the latest peers or removing defunct peers from the cluster
*/
package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
	api "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	weaveapi "github.com/rajch/weave/api"
	"github.com/rajch/weave/common"
)

type nodeInfo struct {
	name string
	addr string
}

// return the IP addresses of all nodes in the cluster
func getKubePeers(c kubernetes.Interface, includeWithNoIPAddr bool) ([]nodeInfo, error) {
	ctx := context.Background()
	nodeList, err := c.CoreV1().Nodes().List(ctx, api.ListOptions{})
	if err != nil {
		return nil, err
	}
	addresses := make([]nodeInfo, 0, len(nodeList.Items))
	for _, peer := range nodeList.Items {
		var internalIP, externalIP string
		for _, addr := range peer.Status.Addresses {
			// Check it's a valid ipv4 address
			ip := net.ParseIP(addr.Address)
			if ip == nil || ip.To4() == nil {
				continue
			}
			if addr.Type == "InternalIP" {
				internalIP = ip.To4().String()
			} else if addr.Type == "ExternalIP" {
				externalIP = ip.To4().String()
			}
		}

		// Fallback for cases where a Node has an ExternalIP but no InternalIP
		if internalIP != "" {
			// exclude self from the list of peers this node will peer with
			if isLocalNodeIP(internalIP) {
				continue
			}
			addresses = append(addresses, nodeInfo{name: peer.Name, addr: internalIP})
		} else if externalIP != "" {
			addresses = append(addresses, nodeInfo{name: peer.Name, addr: externalIP})
		} else if includeWithNoIPAddr {
			addresses = append(addresses, nodeInfo{name: peer.Name, addr: ""})
		}
	}
	return addresses, nil
}

// returns true if given IP matches with one of the local IP's
func isLocalNodeIP(ip string) bool {
	addrs, err := netlink.AddrList(nil, unix.AF_INET)
	if err != nil {
		return false
	}
	for _, addr := range addrs {
		if addr.Peer != nil && addr.Peer.IP.String() == ip {
			return true
		}
	}
	return false
}

// (minimal, incomplete) interface so weaver can be mocked for testing.
type weaveClient interface {
	RmPeer(peerName string) (string, error)
}

// For each of those peers that is no longer listed as a node by
// Kubernetes, remove it from Weave IPAM
func reclaimRemovedPeers(kube kubernetes.Interface, cml *configMapAnnotations, myPeerName, myNodeName string) error {
	weave := weaveapi.NewClient(os.Getenv("WEAVE_HTTP_ADDR"), common.Log)
	for loopsWhenNothingChanged := 0; loopsWhenNothingChanged < 3; loopsWhenNothingChanged++ {
		// 1. Compare peers stored in the peerList against all peers reported by k8s now.
		storedPeerList, err := getRefreshedPeerList(cml)
		if err != nil {
			return err
		}
		nodes, err := getKubePeers(kube, true)
		if err != nil {
			return err
		}
		nodeSet := make(map[string]struct{}, len(nodes))
		for _, node := range nodes {
			nodeSet[node.name] = struct{}{}
		}
		peerMap := make(map[string]peerInfo, len(storedPeerList.Peers))
		for _, peer := range storedPeerList.Peers {
			if peer.PeerName != myPeerName {
				peerMap[peer.PeerName] = peer
			}
		}
		// remove entries from the peer map that are current nodes
		for key, peer := range peerMap {
			if _, found := nodeSet[peer.NodeName]; found {
				// unless they have a duplicate of my NodeName but are not me
				if peer.NodeName == myNodeName && peer.PeerName != myPeerName {
					continue
				}
				delete(peerMap, key)
			}
		}
		// so the remainder is everything we want to clean up
		common.Log.Debugln("[kube-peers] Nodes that have disappeared:", peerMap)
		if len(peerMap) == 0 {
			break
		}
		// 2. Loop for each X in the first set and not in the second - we wish to remove X from our data structures
		for _, peer := range peerMap {
			if peer.PeerName == myPeerName { // Don't remove myself.
				common.Log.Warnln("[kube-peers] not removing myself", peer)
				continue
			}
			changed, err := reclaimPeer(weave, cml, peer.PeerName, myPeerName)
			if err != nil {
				return err
			}
			if changed {
				loopsWhenNothingChanged = 0
			}
		}

		// 9. Go back to step 1 until there is no difference between the two sets
		// (or we hit the counter that says we've been round the loop 3 times and nothing happened)
	}
	return nil
}

// Attempt to reclaim the IP addresses owned by peerName, using the
// Kubernetes api-server as a point of consensus so that only one peer
// actions the reclaim.
// Return a bool to show whether we attempted to change anything,
// and an error if something went wrong.
func reclaimPeer(weave weaveClient, cml *configMapAnnotations, peerName string, myPeerName string) (changed bool, err error) {
	ctx := context.Background()
	common.Log.Debugln("[kube-peers] Preparing to remove disappeared peer", peerName)
	okToRemove := false
	nonExistentPeer := false

	// Re-read status from Kubernetes; speculative updates may leave it un-set
	if err := cml.Init(); err != nil {
		return false, err
	}

	// 3. Check if there is an existing annotation with key X
	existingAnnotation, found := cml.GetAnnotation(KubePeersPrefix + peerName)
	if found {
		common.Log.Debugln("[kube-peers] Existing annotation", existingAnnotation)
		// 4.   If annotation already contains my identity, ok;
		if existingAnnotation == myPeerName {
			okToRemove = true
		} else {
			storedPeerList, err := cml.GetPeerList()
			if err != nil {
				return false, err
			}
			// handle an edge case where peer claimed to own the action to reclaim but no longer
			// exists hence lock persists foever
			if !storedPeerList.contains(existingAnnotation) {
				nonExistentPeer = true
				common.Log.Debugln("[kube-peers] Existing annotation", existingAnnotation, " has a non-existent peer so owning the reclaim action")
			}
		}
	}
	if !found || nonExistentPeer {
		// 5.   If non-existent, write an annotation with key X and contents "my identity"
		common.Log.Debugln("[kube-peers] Noting I plan to remove ", peerName)
		if err := cml.UpdateAnnotation(ctx, KubePeersPrefix+peerName, myPeerName); err == nil {
			okToRemove = true
		} else {
			common.Log.Errorln("[kube-peers] error from UpdateAnnotation: ", err)
		}
	}
	if !okToRemove {
		return false, nil
	}
	// 6.   If step 4 or 5 succeeded, rmpeer X
	// we will try once more to see if the peerName has been removed from the annotation
	// by another weave pod
	storedPeerList, err := getRefreshedPeerList(cml)
	if storedPeerList.contains(peerName) {
		result, err := weave.RmPeer(peerName)
		common.Log.Infof("[kube-peers] rmpeer of %s: %s", peerName, result)
		if err != nil {
			return false, err
		}
	}
	err = cml.LoopUpdate(func() error {
		// 7aa.   Remove any annotations Z* that have contents X
		if err := cml.RemoveAnnotationsWithValue(ctx, peerName); err != nil {
			return err
		}
		// 7a.    Remove X from peerList
		storedPeerList, err := cml.GetPeerList()
		if err != nil {
			return err
		}
		if storedPeerList.contains(peerName) {
			storedPeerList.remove(peerName)
			if err := cml.UpdatePeerList(ctx, *storedPeerList); err != nil {
				return err
			}
		} else {
			common.Log.Infof("[kube-peers] annotation was removed already for %s", peerName)
		}
		// 7b.    Remove annotation with key X
		return cml.RemoveAnnotation(ctx, KubePeersPrefix+peerName)
	})
	// 8.   If step 5 failed due to optimistic lock conflict, stop: someone else is handling X

	// Step 3-5 is to protect against two simultaneous rmpeers of X
	// Step 4 is to pick up again after a restart between step 5 and step 7b
	// If the peer doing the reclaim disappears between steps 5 and 7a, then someone will clean it up in step 7aa
	// If peer doing the reclaim disappears forever between 7a and 7b then we get a dangling annotation
	// This should be sufficiently rare that we don't care.

	// Question: Should we narrow step 2 by checking against Weave Net IPAM?
	// i.e. If peer X owns any address space and is marked unreachable, we want to rmpeer X
	return true, err
}

// resetPeers replaces the peers list with current set of peers
func resetPeers(kube kubernetes.Interface) error {
	nodes, err := getKubePeers(kube, false)
	if err != nil {
		return err
	}
	peerList := make([]string, 0)
	for _, node := range nodes {
		peerList = append(peerList, node.addr)
	}
	weave := weaveapi.NewClient(os.Getenv("WEAVE_HTTP_ADDR"), common.Log)
	err = weave.ReplacePeers(peerList)
	if err != nil {
		return err
	}
	return nil
}

// regiesters with Kubernetes API server for node delete events. Node delete event handler
// invokes reclaimRemovedPeers to remove it from IPAM so that IP space is reclaimed
func registerForNodeUpdates(client *kubernetes.Clientset, stopCh <-chan struct{}, nodeName, peerName string) {
	informerFactory := informers.NewSharedInformerFactory(client, 0)
	nodeInformer := informerFactory.Core().V1().Nodes().Informer()
	common.Log.Debugln("registering for updates for node delete events")
	nodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		DeleteFunc: func(obj interface{}) {
			var name string
			if apiObj, ok := obj.(api.Object); ok {
				name = apiObj.GetName()
			} else {
				name = fmt.Sprintf("%#v", obj)
			}
			common.Log.Debugln("Delete event for", name)
			// add random delay to avoid all nodes acting on node delete event at the same
			// time leading to contention to use `weave-net` configmap
			r := rand.Intn(5000)
			time.Sleep(time.Duration(r) * time.Millisecond)

			cml := newConfigMapAnnotations(configMapNamespace, configMapName, client)
			err := reclaimRemovedPeers(client, cml, peerName, nodeName)
			if err != nil {
				common.Log.Fatalf("[kube-peers] Error while reclaiming space: %v", err)
			}
			err = resetPeers(client)
			if err != nil {
				common.Log.Fatalf("[kube-peers] Error resetting peer list: %v", err)
			}
		},
	})
	informerFactory.WaitForCacheSync(stopCh)
	informerFactory.Start(stopCh)
}

// getRefreshedPeerList will fetch the latest peer list from the config map annotation
func getRefreshedPeerList(cml *configMapAnnotations) (*peerList, error) {
	if err := cml.Init(); err != nil {
		return nil, err
	}
	storedPeerList, err := cml.GetPeerList()
	if err != nil {
		return nil, err
	}
	return storedPeerList, nil
}

func main() {
	var (
		justReclaim       bool
		justCheck         bool
		justSetNodeStatus bool
		justVersion       bool
		justUID           bool
		peerName          string
		nodeName          string
		logLevel          string
		runReclaimDaemon  bool
	)
	flag.BoolVar(&justReclaim, "reclaim", false, "reclaim IP space from dead peers")
	flag.BoolVar(&runReclaimDaemon, "run-reclaim-daemon", false, "run background process that reclaim IP space from dead peers ")
	flag.BoolVar(&justCheck, "check-peer-new", false, "return success if peer name is not stored in annotation")
	flag.BoolVar(&justSetNodeStatus, "set-node-status", false, "set NodeNetworkUnavailable to false")
	flag.BoolVar(&justVersion, "print-k8s-version", false, "print the Kubernetes version and exit")
	flag.BoolVar(&justUID, "print-uid", false, "print a UID for this installation and exit")
	flag.StringVar(&peerName, "peer-name", "unknown", "name of this Weave Net peer")
	flag.StringVar(&nodeName, "node-name", "unknown", "name of this Kubernetes node")
	flag.StringVar(&logLevel, "log-level", "info", "logging level (debug, info, warning, error)")
	flag.Parse()

	common.SetLogLevel(logLevel)
	config, err := rest.InClusterConfig()
	if err != nil {
		common.Log.Fatalf("[kube-peers] Could not get cluster config: %v", err)
	}
	c, err := kubernetes.NewForConfig(config)
	if err != nil {
		common.Log.Fatalf("[kube-peers] Could not make Kubernetes connection: %v", err)
	}
	if justCheck {
		cml := newConfigMapAnnotations(configMapNamespace, configMapName, c)
		exists, err := checkIamInPeerList(cml, c, peerName)
		if err != nil {
			common.Log.Fatalf("[kube-peers] Could not check peer list: %v", err)
		}
		if exists {
			os.Exit(9)
		} else {
			os.Exit(0)
		}
	}
	if justSetNodeStatus {
		err := setNodeNetworkUnavailableFalse(c, nodeName)
		if err != nil {
			common.Log.Fatalf("[kube-peers] could not set node status: %v", err)
		}
		return
	}
	if justReclaim {
		cml := newConfigMapAnnotations(configMapNamespace, configMapName, c)

		list, err := addMyselfToPeerList(cml, c, peerName, nodeName)
		if err != nil {
			common.Log.Fatalf("[kube-peers] Could not update peer list: %v", err)
		}
		common.Log.Infoln("[kube-peers] Added myself to peer list", list)

		err = reclaimRemovedPeers(c, cml, peerName, nodeName)
		if err != nil {
			common.Log.Fatalf("[kube-peers] Error while reclaiming space: %v", err)
		}
		return
	}
	if justVersion {
		k8sVersion, err := c.Discovery().ServerVersion()
		if err != nil {
			common.Log.Fatalf("[kube-peers] Could not get Kubernetes version: %v", err)
		}
		fmt.Println(k8sVersion.String())
	}
	if justUID {
		// use UID of `weave-net` configmap as unique ID of the Kubernetes cluster
		cm, err := c.CoreV1().ConfigMaps(configMapNamespace).Get(context.Background(), configMapName, api.GetOptions{})
		if err != nil {
			common.Log.Fatalf("Unable to fetch ConfigMap %s/%s to infer unique cluster ID", configMapNamespace, configMapName)
		}
		fmt.Println(cm.ObjectMeta.UID)
	}
	if runReclaimDaemon {
		// Handle SIGINT and SIGTERM
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		stopCh := make(chan struct{})
		rand.Seed(time.Now().UnixNano())
		registerForNodeUpdates(c, stopCh, nodeName, peerName)
		<-ch
		close(stopCh)
		return
	}

	peers, err := getKubePeers(c, false)
	if err != nil {
		common.Log.Fatalf("[kube-peers] Could not get peers: %v", err)
	}
	for _, node := range peers {
		fmt.Println(node.addr)
	}
}
