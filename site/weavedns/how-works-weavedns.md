---
title: How Weave Finds Containers
layout: default
---


The weavedns service running on every host acts as the nameserver for
containers on that host. It learns about hostnames for local containers
from the proxy and from the `weave run` command.  

If a hostname is in the `.weave.local` domain, then weavedns records the association of that
name with the container's Weave IP address(es) in its in-memory
database, and then broadcasts the association to other Weave peers in the
cluster.

When weavedns is queried for a name in the `.weave.local` domain, it
looks up the hostname in its memory database and responds with the IPs
of all containers for that hostname across the entire cluster.

###Basic Load Balancing and Fault Tolerance

Weavedns returns IP addresses in random order to facilitate basic
load balancing and failure tolerance. Most client side resolvers sort
the returned addresses based on reachability, and place local addresses
at the top of the list (see [RFC 3484](https://www.ietf.org/rfc/rfc3484.txt)).

For example, if there is container with the desired hostname on the local
machine, the application receives that container's IP address.
Otherwise, the application receives the IP address of a random
container with the desired hostname.

When weavedns is queried for a name in a domain other than
`.weave.local`, it queries the host's configured nameserver, which is
the standard behaviour for Docker containers.

###Specifying a Different Docker Bridge Device

So that containers can connect to a stable and always routable IP
address, weavedns listens on port 53 to the Docker bridge device, which
is assumed to be `docker0`.  Some configurations may use a different
Docker bridge device. To supply a different bridge device, use the
environment variable `DOCKER_BRIDGE`, e.g.,

```bash
$ sudo DOCKER_BRIDGE=someother weave launch
```

In the event that weavedns is launched in this way, it's important that
other calls to `weave` also specify the bridge device:

```bash
$ sudo DOCKER_BRIDGE=someother weave run ...
```

**See Also**

 * [Using Weavedns](/site/weavedns/overview-using-weavedns.md)
 * [Load Balancing with weavedns](/site/weavedns/load-balance-fault-weavedns.md)
 * [Managing Domain Entries](/site/weavedns/managing-domains-weavedns.md)
