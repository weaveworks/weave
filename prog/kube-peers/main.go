package main

import (
	"flag"
	"fmt"
	"log"
	"net"

	"k8s.io/client-go/kubernetes"
	api "k8s.io/client-go/pkg/api/v1"
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
	if err := cml.Init(); err != nil {
		return nil, err
	}
	list, err := cml.GetPeerList()
	if err != nil {
		return nil, err
	}
	log.Println("Fetched existing peer list", list)
	if !list.contains(peerName) {
		list.add(peerName, name)
		log.Println("Storing new peer list", list)
		err = cml.UpdatePeerList(*list)
		if err != nil {
			return nil, err
		}
	}
	return list, nil
}

// For each of those peers that is no longer listed as a node by
// Kubernetes, remove it from Weave IPAM
func reclaimRemovedPeers(apl *peerList, nodes []nodeInfo) error {
	// TODO
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
