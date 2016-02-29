---
title: How the Weave Docker Network Plugin Works
layout: default
---


The Weave plugin actually provides *two* network drivers to Docker - one named `weavemesh` that can operate without a cluster store and another one named `weave` that can only work with one (like Docker's overlay driver).

### `weavemesh` driver

* Weave handles all co-ordination between hosts (referred to by Docker as a "local scope" driver)
* Supports a single network only. A network named `weave` is automatically created for you.
* Uses Weave's partition tolerant IPAM

If you do create additional networks using the `weavemesh` driver, containers attached to them will be able to communicate with containers attached to `weave`. There is no isolation between those networks.

### `weave` driver

* This runs in what Docker calls "global scope", which requires an external cluster store
* Supports multiple networks that must be created using `docker network create --driver weave ...`
* Used with Docker's cluster-store-based IPAM

There's no specific documentation from Docker on using a cluster
store, but the first part of
[Getting Started with Docker Multi-host Networking](https://github.com/docker/docker/blob/master/docs/userguide/networking/get-started-overlay.md) is a good place to start.

>>**Note:** In the case of multiple networks using the `weave` driver, all containers are on the same virtual network but Docker allocates their addresses on different subnets so they cannot talk to each other directly.


**See Also**

 * [Using the Weave Net Docker Network Plugin](/site/plugin/weave-plugin-how-to.md)
 * [Plugin Command-line Arguments](/site/plugin/plug-in-command-line.md)
 
