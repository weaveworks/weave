---
title: Weave Net Tasks
menu_order: 50
search_type: Documentation
---

The section describes the configuration options available in Weave Net. It is divided into: Managing Containerized Applications, Administering IP Addresses, Managing WeaveDNS, and finally attaching Docker Containers via the Weave API Proxy. 


## Managing Containerized Applications


* [Isolating Applications on a Weave Network]({{ '/tasks/manage/application-isolation' | relative_url }})
* [Dynamically Attaching and Detaching Applications]({{ '/tasks/manage/dynamically-attach-containers' | relative_url }})
* [Integrating with the Host Network]({{ '/tasks/manage/host-network-integration' | relative_url }})
* [Managing Services - Exporting, Importing, Binding and Routing]({{ '/tasks/manage/service-management' | relative_url }})
     * [Exporting Services]({{ '/tasks/manage/service-management#exporting' | relative_url }})
     * [Importing Services]({{ '/tasks/manage/service-management#importing' | relative_url }})
     * [Binding Services]({{ '/tasks/manage/service-management#binding' | relative_url }})
     * [Routing Services]({{ '/tasks/manage/service-management#routing' | relative_url }})
     * [Dynamically Changing Service Locations]({{ '/tasks/manage/service-management#change-location' | relative_url }})
* [Securing Connections Across Untrusted Networks]({{ '/tasks//manage/security-untrusted-networks' | relative_url }})
* [Adding and Removing Hosts Dynamically]({{ '/tasks/manage/finding-adding-hosts-dynamically' | relative_url }})
* [Using Fast Datapath]({{ '/tasks/manage/fastdp' | relative_url }})
* [Enabling Multi-Cloud, Multi-Hop Networking and Routing]({{ '/tasks/manage/multi-cloud-multi-hop' | relative_url }})
* [Configuring IP Routing on an Amazon Web Services Virtual Private Cloud]({{ '/tasks/manage/awsvpc' | relative_url }})
* [Monitoring with Prometheus]({{ '/tasks/manage/metrics' | relative_url }})


## Administering IP Addresses


* [Allocating IP Addresses]({{ '/tasks/ipam/ipam' | relative_url }})
     * [Initializing Peers on a Weave Network]({{ '/tasks/ipam/ipam#initialization' | relative_url }})
     * [--ipalloc-init:consensus and How Quorum is Achieved]({{ '/tasks/ipam/ipam#quorum' | relative_url }})
     * [Priming a Peer]({{ '/tasks/ipam/ipam#priming-a-peer' | relative_url }})
     * [Choosing an Allocation Range]({{ '/tasks/ipam/ipam#range' | relative_url }})
* [Allocating IPs in a Specific Range]({{ '/tasks/ipam/configuring-weave' | relative_url }})
* [Manually Specifying the IP Address of a Container]({{ '/tasks/ipam/manual-ip-address' | relative_url }})
* [Automatic Allocation Across Multiple Subnets]({{ '/tasks/ipam/allocation-multi-ipam' | relative_url }})
* [Starting, Stopping and Removing Peers]({{ '/tasks/ipam/stop-remove-peers-ipam' | relative_url }})
* [Troubleshooting the IP Allocator]({{ '/tasks/ipam/troubleshooting-ipam' | relative_url }})


## Managing WeaveDNS


* [Discovering Containers with WeaveDNS]({{ '/tasks/weavedns/weavedns' | relative_url }})
* [How Weave Finds Containers]({{ '/tasks/weavedns/how-works-weavedns' | relative_url }})
* [Load Balancing and Fault Resilience with weaveDNS]({{ '/tasks/weavedns/load-balance-fault-weavedns' | relative_url }})
* [Managing Domains]({{ '/tasks/weavedns/managing-domains-weavedns' | relative_url }})
     * [Configuring the domain search path]({{ '/tasks/weavedns/managing-domains-weavedns#domain-search-path' | relative_url }})
     * [Using a different local domain]({{ '/tasks/weavedns/managing-domains-weavedns#local-domain' | relative_url }})
* [Managing Domain Entries]({{ '/tasks/weavedns/managing-entries-weavedns' | relative_url }})
     * [Adding and removing extra DNS entries]({{ '/tasks/weavedns/managing-entries-weavedns#add-remove' | relative_url }})
     * [Resolving WeaveDNS entries from the Host]({{ '/tasks/weavedns/managing-entries-weavedns#resolve-weavedns-entries-from-host' | relative_url }})
     * [Hot-swapping Service Containers]({{ '/tasks/weavedns/managing-entries-weavedns#hot-swapping' | relative_url }})
     * [Configuring a Custom TTL]({{ '/tasks/weavedns/managing-entries-weavedns#ttl' | relative_url }})
* [Troubleshooting and Present Limitations]({{ '/tasks/weavedns/troubleshooting-weavedns' | relative_url }})


## Attaching Docker Containers via the Weave API Proxy


* [Integrating Docker via the API Proxy]({{ '/tasks/weave-docker-api/weave-docker-api' | relative_url }})
* [Using The Weave Docker API Proxy]({{ '/tasks/weave-docker-api/using-proxy' | relative_url }})
* [Securing the Docker Communications With TLS]({{ '/tasks/weave-docker-api/securing-proxy' | relative_url }})
* [Automatic IP Allocation and the Weave Proxy]({{ '/tasks/weave-docker-api/ipam-proxy' | relative_url }})
* [Using Automatic Discovery With the Weave Net Proxy]({{ '/tasks/weave-docker-api/automatic-discovery-proxy' | relative_url }})
* [Name resolution via `/etc/hosts`]({{ '/tasks/weave-docker-api/name-resolution-proxy' | relative_url }})
* [Launching Containers With Weave Run (without the Proxy)]({{ '/tasks/weave-docker-api/launching-without-proxy' | relative_url }})
