---
title: Weave Net Tasks
menu_order: 50
search_type: Documentation
---

The section describes the configuration options available in Weave Net. It is divided into: Managing Containerized Applications, Administering IP Addresses, Managing WeaveDNS, and finally attaching Docker Containers via the Weave API Proxy. 

## Managing Containerized Applications

* [Isolating Applications on a Weave Network]
* [Dynamically Attaching and Detaching Applications]
* [Integrating with the Host Network]
* [Managing Services - Exporting, Importing, Binding and Routing]
     * [Exporting Services](https://weave.works/docs/net/latest/tasks/manage/service-management/#exporting)
     * [Importing Services](https://weave.works/docs/net/latest/tasks/manage/service-management/#importing)
     * [Binding Services](https://weave.works/docs/net/latest/tasks/manage/service-management/#binding)
     * [Routing Services](https://weave.works/docs/net/latest/tasks/manage/service-management/#routing)
     * [Dynamically Changing Service Locations](https://weave.works/docs/net/latest/tasks/manage/service-management/#change-location)
* [Securing Connections Across Untrusted Networks]
* [Adding and Removing Hosts Dynamically]
* [Using Fast Datapath]
* [Enabling Multi-Cloud, Multi-Hop Networking and Routing]
* [Configuring IP Routing on an Amazon Web Services Virtual Private Cloud]
* [Monitoring with Prometheus]


## Administering IP Addresses

* [Allocating IP Addresses]
     * [Initializing Peers on a Weave Network](https://weave.works/docs/net/latest/tasks/ipam/ipam/#initialization)
     * [--ipalloc-init:consensus and How Quorum is Achieved](https://weave.works/docs/net/latest/tasks/ipam/ipam/#quorum)
     * [Priming a Peer](https://weave.works/docs/net/latest/tasks/ipam/ipam/#priming-a-peer)
     * [Choosing an Allocation Range](https://weave.works/docs/net/latest/tasks/ipam/ipam/#range)
* [Allocating IPs in a Specific Range]
* [Manually Specifying the IP Address of a Container]
* [Automatic Allocation Across Multiple Subnets]
* [Starting, Stopping and Removing Peers]
* [Troubleshooting the IP Allocator]


## Managing WeaveDNS

* [Discovering Containers with WeaveDNS]
* [How Weave Finds Containers]
* [Load Balancing and Fault Resilience with weaveDNS]
* [Managing Domains]
     * [Configuring the domain search path](https://weave.works/docs/net/latest/tasks/weavedns/managing-domains-weavedns/#domain-search-path)
     * [Using a different local domain](https://weave.works/docs/net/latest/tasks/weavedns/managing-domains-weavedns/#local-domain)
* [Managing Domain Entries]
     * [Adding and removing extra DNS entries](https://weave.works/docs/net/latest/tasks/weavedns/managing-entries-weavedns/#add-remove)
     * [Resolving WeaveDNS entries from the Host](https://weave.works/docs/net/latest/tasks/weavedns/managing-entries-weavedns/#resolve-weavedns-entries-from-host)
     * [Hot-swapping Service Containers](https://weave.works/docs/net/latest/tasks/weavedns/managing-entries-weavedns/#hot-swapping)
     * [Configuring a Custom TTL](https://weave.works/docs/net/latest/tasks/weavedns/managing-entries-weavedns/#ttl)
* [Troubleshooting and Present Limitations]

## Attaching Docker Containers via the Weave API Proxy

* [Integrating Docker via the API Proxy]
* [Using The Weave Docker API Proxy]
* [Securing the Docker Communications With TLS]
* [Automatic IP Allocation and the Weave Proxy]
* [Using Automatic Discovery With the Weave Net Proxy]
* [Name resolution via `/etc/hosts`]
* [Launching Containers With Weave Run (without the Proxy)]
