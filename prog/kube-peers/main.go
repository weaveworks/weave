package main

import (
	"flag"
	"fmt"
	"log"
	"net"

	api "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type nodeInfo struct {
	name string
	addr string
}

// return the IP addresses of all nodes in the cluster
func getKubePeers(c *kubernetes.Clientset) ([]nodeInfo, error) {
	nodeList, err := c.Nodes().List(api.ListOptions{})
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
		}
	}
	return addresses, nil
}

const (
	configMapName      = "weave-net"
	configMapNamespace = "kube-system"
)

// update the list of all peers that have gone through this code path
func addMyselfToPeerList(c *kubernetes.Clientset, peerName, name string) (*peerList, error) {
	cml := newConfigMapAnnotations(configMapNamespace, configMapName, c)
	var list *peerList
	err := cml.LoopUpdate(func() error {
		var err error
		list, err = cml.GetPeerList()
		if err != nil {
			return err
		}
		log.Println("Fetched existing peer list", list)
		if !list.contains(peerName) {
			list.add(peerName, name)
			log.Println("Storing new peer list", list)
			err = cml.UpdatePeerList(*list)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return list, err
}

// For each of those peers that is no longer listed as a node by
// Kubernetes, remove it from Weave IPAM
func reclaimRemovedPeers(apl *peerList, nodes []nodeInfo) error {
	// TODO
	// Outline of function:
	// 1. Compare peers stored in the peerList against all peers reported by k8s now.
	// 2. Loop for each X in the first set and not in the second - we wish to remove X from our data structures
	// 3. Check if there is an existing annotation with key X
	// 4.   If annotation already contains my identity, ok;
	// 5.   If non-existent, write an annotation with key X and contents "my identity"
	// 6.   If step 4 or 5 succeeded, rmpeer X
	// 7aa.   Remove any annotations Z* that have contents X
	// 7a.    Remove X from peerList
	// 7b.    Remove annotation with key X
	// 8.   If step 5 failed due to optimistic lock conflict, stop: someone else is handling X
	// 9. Go back to step 1 until there is no difference between the two sets

	// Step 3-5 is to protect against two simultaneous rmpeers of X
	// Step 4 is to pick up again after a restart between step 5 and step 7b
	// If the peer doing the reclaim disappears between steps 5 and 7a, then someone will clean it up in step 7aa
	// If peer doing the reclaim disappears forever between 7a and 7b then we get a dangling annotation
	// This should be sufficiently rare that we don't care.

	// Question: Should we narrow step 2 by checking against Weave Net IPAM?
	// i.e. If peer X owns any address space and is marked unreachable, we want to rmpeer X
	return nil
}

func main() {
	var (
		justReclaim bool
		peerName    string
		nodeName    string
	)
	flag.BoolVar(&justReclaim, "reclaim", false, "reclaim IP space from dead peers")
	flag.StringVar(&peerName, "peer-name", "unknown", "name of this Weave Net peer")
	flag.StringVar(&nodeName, "node-name", "unknown", "name of this Kubernetes node")
	flag.Parse()

	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("Could not get cluster config: %v", err)
	}
	c, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Could not make Kubernetes connection: %v", err)
	}
	peers, err := getKubePeers(c)
	if err != nil {
		log.Fatalf("Could not get peers: %v", err)
	}
	if justReclaim {
		log.Println("Checking if any peers need to be reclaimed")
		list, err := addMyselfToPeerList(c, peerName, nodeName)
		if err != nil {
			log.Fatalf("Could not get peer list: %v", err)
		}
		err = reclaimRemovedPeers(list, peers)
		if err != nil {
			log.Fatalf("Error while reclaiming space: %v", err)
		}
		return
	}
	for _, node := range peers {
		fmt.Println(node.addr)
	}
}
