---
title: FAQ
menu_order: 100
---



**Q: How do I obtain the IP of a specific container when I'm using Weave?**

You can use `weave ps <container>` to see the allocated address of a container on a Weave network.  

See [Troubleshooting Weave - List attached containers](/site/troubleshooting.md#list-attached-containers).


**Q: My dockerized app needs to check the request of an application that uses a static IP. Is it possible to manually change the IP of a container?**


You can manually change the IP of a container using [Classless Inter-Domain Routing or CIDR notation](https://en.wikipedia.org/wiki/Classless_Inter-Domain_Routing). 

For more information, refer to [Manually Specifying the IP Address of a Container](/site/using-weave/manual-ip-address.md). 


**Q: How do I expose one of my containers to the outside world?**

Exposing a container to the outside world is described in [Exporting Services](/site/using-weave/service-management.md#exporting).


**Q: Can I connect my existing 'legacy' network with a Weave container network?**

Yes you can. 

For example, you have a Weave network that runs on hosts A, B, C. and you have an additional host, that we'll call P, where neither Weave nor Docker are running.  However, you need to connect from a process running on host P to a specific container running on host B on the Weave network.  Since the Weave network is completely separate from any network that P is connected to, you cannot connect the container using the container's IP address. 

A simple way to accomplish this would be to run Weave on the host and then run, `weave expose` to expose the network to any running containers. Or you set up a route from P to one of A, B or C. See [Integrating a Network Host](/site/using-weave/host-network-integration.md).

Yet another option is to expose a port from the container on host B and then connect to it. You can read about exposing ports in [Exporting Services](/site/using-weave/service-management.md#exporting).


**Q: Why am I seeing the same IP address assigned to two different containers on different hosts?**

Under normal circumstances, this should never happen, but it can occur if  `weave forget` and `weave rmpeer` was run on more than one host. 

You cannot call `weave rmpeer` on more than one host. The address space, which was owned by the stale peer cannot be left dangling, and as a result it gets reassigned. In this instance, the address is reassigned to the peer on which `weave rmpeer` was run. Therefore, if you run `weave forget` and then `weave rmpeer` on more than one host at a time, it results in duplicate IPs on more than one host.

Once the peers detect the inconsistency, they log the error and drop the connection that supplied the inconsistent data. The rest of the peers will carry on with their view of the world, but the network will not function correctly.

Some peers may be able to communicate their claim to the others before they run `rmpeer` (i.e. it's a race), so what you can expect is a few cliques of peers that are still talking to each other, but repeatedly dropping attempted connections with peers in other cliques.

For more information on see [Allocating IP Addresses](/site/ipam.md) and also, [Starting, Stopping and Removing Peers](/site/ipam/stop-remove-peers-ipam.md).


**Q: What is the best practice for resetting a node that goes out of service?**

When a node goes out of service, the best option is to call `weave rmpeer` on one host and then `weave forget` on all the other hosts.

See [Starting, Stopping and Removing Peers](/site/ipam/stop-remove-peers-ipam.md) for an in-depth discussion.


**Q: What about Weave's performance? Are software defined network overlays just as fast as native networking?**

All virtualization techniques have some overhead, and Weave's overhead is typically around 2-3%. Unless your system is completely bottlenecked on the network, you won't notice this during normal operation. 

Weave Net also automatically uses the fastest datapath between two hosts. When Weave Net can't use the fast datapath between two hosts, it falls back to the slower packet forwarding approach. Selecting the fastest forwarding approach is automatic, and is determined on a connection-by-connection basis. For example, a Weave network spanning two data centers might use fast datapath within the data centers, but not for the more constrained network link between them.

For more information about fast datapath see [How Fast Datapath Works](/site/how-it-works/fastdp-how-it-works.md).


**Q: How can I tell if Weave is using fast datapath (fastdp) or not?**

To view whether Weave is using fastdp or not, you can run, `weave status connections`

For more information on this command, see [Using Fast Datapath](/site/using-weave/fastdp.md).


**Q: Does encryption work with fastdp?**

Encryption does not work with fast datapath. If you enable encryption using the `--password` option to launch Weave (or you use the `WEAVE_PASSWORD` environment variable), fast datapath will by default be disabled. 

You can however have a mixture of fast datapath connections over trusted links, as well as, encrypted connections over untrusted links.

See [Using Fast Datapath](/site/using-weave/fastdp.md) for more information.

**Q: Can I create multiple networks where containers can communicate on one network, but are isolated from containers on other networks?**

Yes, of course!  Weave allows you to run isolated networks and still allow open communications between individual containers from those isolated networks. You can find information on how to do this in [Application Isolation](/site/using-weave/application-isolation.md).


**Q: Which ports does Weave Net use (e.g. if I am configuring a firewall) ?**

You must permit traffic to flow through TCP 6783 and UDP 6783/6784,
which are Weave’s control and data ports.

The daemon also uses TCP port 6782 for [metrics](/site/metrics.md), but
you would only need to open up this port if you wish to collect metrics
from another host.

The Weave Net daemon listens on localhost (127.0.0.1) TCP port 6784
for commands from other Weave Net components. This port should not be
opened to other hosts.

**<a name=own-image></a>Q: Why do you use your own Docker image `weaveworks/ubuntu`?**

The official Ubuntu image does not contain the `ping` and `nc`
commands which are used in many of our examples throughout the
documentation. The `weaveworks/ubuntu` image is simply the official
Ubuntu image with those two commands added.


**See Also**

 * [Troubleshooting Weave](/site/troubleshooting.md)
 * [Troubleshooting IPAM](/site/ipam.md)
 * [Troubleshooting the Proxy](/site/weave-docker-api/using-proxy.md)
 
