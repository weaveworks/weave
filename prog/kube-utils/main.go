/*
Package main deals with weave-net peers on the cluster.

This involves peer management, such as getting the latest peers or removing defunct peers from the cluster
*/
package main

import (
	"flag"
	"fmt"
	"net"
	"os"

	api "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	weaveapi "github.com/weaveworks/weave/api"
	"github.com/weaveworks/weave/common"
)

type nodeInfo struct {
	name string
	addr string
}

// return the IP addresses of all nodes in the cluster
func getKubePeers(c *kubernetes.Clientset, includeWithNoIPAddr bool) ([]nodeInfo, error) {
	nodeList, err := c.CoreV1().Nodes().List(api.ListOptions{})
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
			addresses = append(addresses, nodeInfo{name: peer.Name, addr: internalIP})
		} else if externalIP != "" {
			addresses = append(addresses, nodeInfo{name: peer.Name, addr: externalIP})
		} else if includeWithNoIPAddr {
			addresses = append(addresses, nodeInfo{name: peer.Name, addr: ""})
		}
	}
	return addresses, nil
}

const (
	configMapName      = "weave-net"
	configMapNamespace = "kube-system"
)

// update the list of all peers that have gone through this code path
func addMyselfToPeerList(cml *configMapAnnotations, c *kubernetes.Clientset, peerName, name string) (*peerList, error) {
	var list *peerList
	err := cml.LoopUpdate(func() error {
		var err error
		list, err = cml.GetPeerList()
		if err != nil {
			return err
		}
		if !list.contains(peerName) {
			list.add(peerName, name)
			err = cml.UpdatePeerList(*list)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return list, err
}

func checkIamInPeerList(cml *configMapAnnotations, c *kubernetes.Clientset, peerName string) (bool, error) {
	if err := cml.Init(); err != nil {
		return false, err
	}
	list, err := cml.GetPeerList()
	if err != nil {
		return false, err
	}
	common.Log.Debugf("[kube-peers] Checking peer %q against list %v", peerName, list)
	return list.contains(peerName), nil
}

// For each of those peers that is no longer listed as a node by
// Kubernetes, remove it from Weave IPAM
func reclaimRemovedPeers(weave *weaveapi.Client, cml *configMapAnnotations, nodes []nodeInfo, myPeerName string) error {
	for loopsWhenNothingChanged := 0; loopsWhenNothingChanged < 3; loopsWhenNothingChanged++ {
		if err := cml.Init(); err != nil {
			return err
		}
		// 1. Compare peers stored in the peerList against all peers reported by k8s now.
		storedPeerList, err := cml.GetPeerList()
		if err != nil {
			return err
		}
		peerMap := make(map[string]peerInfo, len(storedPeerList.Peers))
		for _, peer := range storedPeerList.Peers {
			peerMap[peer.NodeName] = peer
		}
		for _, node := range nodes {
			delete(peerMap, node.name)
		}
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
			common.Log.Debugln("[kube-peers] Preparing to remove disappeared peer", peer)
			okToRemove := false
			// 3. Check if there is an existing annotation with key X
			if existingAnnotation, found := cml.GetAnnotation(KubePeersPrefix + peer.PeerName); found {
				common.Log.Debugln("[kube-peers] Existing annotation", existingAnnotation)
				// 4.   If annotation already contains my identity, ok;
				if existingAnnotation == myPeerName {
					okToRemove = true
				}
			} else {
				// 5.   If non-existent, write an annotation with key X and contents "my identity"
				common.Log.Debugln("[kube-peers] Noting I plan to remove ", peer.PeerName)
				if err := cml.UpdateAnnotation(KubePeersPrefix+peer.PeerName, myPeerName); err == nil {
					okToRemove = true
				} else {
					common.Log.Debugln("[kube-peers] error from UpdateAnnotation: ", err)
				}
			}
			if okToRemove {
				// 6.   If step 4 or 5 succeeded, rmpeer X
				result, err := weave.RmPeer(peer.PeerName)
				common.Log.Infof("[kube-peers] rmpeer of %s: %s", peer.PeerName, result)
				if err != nil {
					return err
				}
				loopsWhenNothingChanged = 0
				cml.LoopUpdate(func() error {
					// 7aa.   Remove any annotations Z* that have contents X
					if err := cml.RemoveAnnotationsWithValue(peer.PeerName); err != nil {
						return err
					}
					// 7a.    Remove X from peerList
					storedPeerList.remove(peer.PeerName)
					if err := cml.UpdatePeerList(*storedPeerList); err != nil {
						return err
					}
					// 7b.    Remove annotation with key X
					return cml.RemoveAnnotation(KubePeersPrefix + peer.PeerName)
				})
			}
			// 8.   If step 5 failed due to optimistic lock conflict, stop: someone else is handling X

			// Step 3-5 is to protect against two simultaneous rmpeers of X
			// Step 4 is to pick up again after a restart between step 5 and step 7b
			// If the peer doing the reclaim disappears between steps 5 and 7a, then someone will clean it up in step 7aa
			// If peer doing the reclaim disappears forever between 7a and 7b then we get a dangling annotation
			// This should be sufficiently rare that we don't care.
		}

		// 9. Go back to step 1 until there is no difference between the two sets
		// (or we hit the counter that says we've been round the loop 3 times and nothing happened)
	}
	// Question: Should we narrow step 2 by checking against Weave Net IPAM?
	// i.e. If peer X owns any address space and is marked unreachable, we want to rmpeer X
	return nil
}

func main() {
	var (
		justReclaim       bool
		justCheck         bool
		justSetNodeStatus bool
		peerName          string
		nodeName          string
		logLevel          string
	)
	flag.BoolVar(&justReclaim, "reclaim", false, "reclaim IP space from dead peers")
	flag.BoolVar(&justCheck, "check-peer-new", false, "return success if peer name is not stored in annotation")
	flag.BoolVar(&justSetNodeStatus, "set-node-status", false, "set NodeNetworkUnavailable to false")
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
	peers, err := getKubePeers(c, justReclaim)
	if err != nil {
		common.Log.Fatalf("[kube-peers] Could not get peers: %v", err)
	}
	if justReclaim {
		cml := newConfigMapAnnotations(configMapNamespace, configMapName, c)

		list, err := addMyselfToPeerList(cml, c, peerName, nodeName)
		if err != nil {
			common.Log.Fatalf("[kube-peers] Could not update peer list: %v", err)
		}
		common.Log.Infoln("[kube-peers] Added myself to peer list", list)

		weave := weaveapi.NewClient(os.Getenv("WEAVE_HTTP_ADDR"), common.Log)
		err = reclaimRemovedPeers(weave, cml, peers, peerName)
		if err != nil {
			common.Log.Fatalf("[kube-peers] Error while reclaiming space: %v", err)
		}
		return
	}
	for _, node := range peers {
		fmt.Println(node.addr)
	}
}
