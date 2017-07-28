---
title: Weave Net Tasks
menu_order: 50
search_type: Documentation
---

The section describes the configuration options available in Weave Net. It is divided into: Managing Containerized Applications, Administering IP Addresses, Managing WeaveDNS, and finally attaching Docker Containers via the Weave API Proxy. 


## Managing Containerized Applications


* [Isolating Applications on a Weave Network](https://weave.works/docs/net/latest/tasks/manage/application-isolation/)
* [Dynamically Attaching and Detaching Applications](https://weave.works/docs/net/latest/tasks/dynamically-attach-containers/)
* [Integrating with the Host Network](https://weave.works/docs/net/latest/tasks/host-network-integration/)
* [Managing Services - Exporting, Importing, Binding and Routing](https://weave.works/docs/net/latest/tasks/manage/service-management/)
     * [Exporting Services](https://weave.works/docs/net/latest/tasks/manage/service-management/#exporting)
     * [Importing Services](https://weave.works/docs/net/latest/tasks/manage/service-management/#importing)
     * [Binding Services](https://weave.works/docs/net/latest/tasks/manage/service-management/#binding)
     * [Routing Services](https://weave.works/docs/net/latest/tasks/manage/service-management/#routing)
     * [Dynamically Changing Service Locations](https://weave.works/docs/net/latest/tasks/manage/service-management/#change-location)
* [Securing Connections Across Untrusted Networks](https://weave.works/docs/net/latest/tasks//manage/security-untrusted-networks/)
* [Adding and Removing Hosts Dynamically](https://weave.works/docs/net/latest/tasks/manage/finding-adding-hosts-dynamically/)
* [Using Fast Datapath](https://weave.works/docs/net/latest/tasks/manage/fastdp/)
* [Enabling Multi-Cloud, Multi-Hop Networking and Routing](https://weave.works/docs/net/latest/tasks/manage/multi-cloud-multi-hop/)
* [Configuring IP Routing on an Amazon Web Services Virtual Private Cloud](https://weave.works/docs/net/latest/tasks/manage/awsvpc/)
* [Monitoring with Prometheus](https://weave.works/docs/net/latest/tasks/manage/metrics/)


## Administering IP Addresses


* [Allocating IP Addresses](https://weave.works/docs/net/latest/tasks/ipam/ipam/)
     * [Initializing Peers on a Weave Network](https://weave.works/docs/net/latest/tasks/ipam/ipam/#initialization)
     * [--ipalloc-init:consensus and How Quorum is Achieved](https://weave.works/docs/net/latest/tasks/ipam/ipam/#quorum)
     * [Priming a Peer](https://weave.works/docs/net/latest/tasks/ipam/ipam/#priming-a-peer)
     * [Choosing an Allocation Range](https://weave.works/docs/net/latest/tasks/ipam/ipam/#range)
* [Allocating IPs in a Specific Range](https://weave.works/docs/net/latest/tasks/ipam/configuring-weave/)
* [Manually Specifying the IP Address of a Container](https://weave.works/docs/net/latest/tasks/ipam/manual-ip-address/)
* [Automatic Allocation Across Multiple Subnets](https://weave.works/docs/net/latest/tasks/ipam/allocation-multi-ipam/)
* [Starting, Stopping and Removing Peers](https://weave.works/docs/net/latest/tasks/ipam/stop-remove-peers-ipam/)
* [Troubleshooting the IP Allocator](https://weave.works/docs/net/latest/tasks/ipam/troubleshooting-ipam/)


## Managing WeaveDNS


* [Discovering Containers with WeaveDNS](https://weave.works/docs/net/latest/tasks/weavedns/weavedns/)
* [How Weave Finds Containers](https://weave.works/docs/net/latest/tasks/weavedns/how-works-weavedns/)
* [Load Balancing and Fault Resilience with weaveDNS](https://weave.works/docs/net/latest/tasks/weavedns/load-balance-fault-weavedns/)
* [Managing Domains](https://weave.works/docs/net/latest/tasks/weavedns/managing-domains-weavedns/)
     * [Configuring the domain search path](https://weave.works/docs/net/latest/tasks/weavedns/managing-domains-weavedns/#domain-search-path)
     * [Using a different local domain](https://weave.works/docs/net/latest/tasks/weavedns/managing-domains-weavedns/#local-domain)
* [Managing Domain Entries](https://weave.works/docs/net/latest/tasks/weavedns/managing-entries-weavedns/)
     * [Adding and removing extra DNS entries](https://weave.works/docs/net/latest/tasks/weavedns/managing-entries-weavedns/#add-remove)
     * [Resolving WeaveDNS entries from the Host](https://weave.works/docs/net/latest/tasks/weavedns/managing-entries-weavedns/#resolve-weavedns-entries-from-host)
     * [Hot-swapping Service Containers](https://weave.works/docs/net/latest/tasks/weavedns/managing-entries-weavedns/#hot-swapping)
     * [Configuring a Custom TTL](https://weave.works/docs/net/latest/tasks/weavedns/managing-entries-weavedns/#ttl)
* [Troubleshooting and Present Limitations](https://weave.works/docs/net/latest/tasks/weavedns/troubleshooting-weavedns/)


## Attaching Docker Containers via the Weave API Proxy


* [Integrating Docker via the API Proxy](https://weave.works/docs/net/latest/tasks/weave-docker-api/weave-docker-api/)
* [Using The Weave Docker API Proxy](https://weave.works/docs/net/latest/tasks/weave-docker-api/using-proxy/)
* [Securing the Docker Communications With TLS](https://weave.works/docs/net/latest/tasks/weave-docker-api/securing-proxy/)
* [Automatic IP Allocation and the Weave Proxy](https://weave.works/docs/net/latest/tasks/weave-docker-api/ipam-proxy/)
* [Using Automatic Discovery With the Weave Net Proxy](https://weave.works/docs/net/latest/tasks/weave-docker-api/automatic-discovery-proxy/)
* [Name resolution via `/etc/hosts`](https://weave.works/docs/net/latest/tasks/weave-docker-api/name-resolution-proxy/)
* [Launching Containers With Weave Run (without the Proxy)](https://weave.works/docs/net/latest/tasks/weave-docker-api/launching-without-proxy/)
