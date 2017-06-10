package main

import (
	"fmt"
	"log"
	"net"

	"k8s.io/client-go/kubernetes"
	api "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
)

func getKubePeers(c *kubernetes.Clientset) ([]string, error) {
	nodeList, err := c.Nodes().List(api.ListOptions{})
	if err != nil {
		return nil, err
	}
	addresses := make([]string, 0, len(nodeList.Items))
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
			addresses = append(addresses, internalIP)
		} else if externalIP != "" {
			addresses = append(addresses, externalIP)
		}
	}
	return addresses, nil
}

func main() {
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
	for _, addr := range peers {
		fmt.Println(addr)
	}
}
