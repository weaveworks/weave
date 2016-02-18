---
title: Weave Net FAQ
layout: default
---



##Q:How do I obtain the IP of a specific container when I'm using Weave?

To see which address was allocated to a container on a Weave network, run: `weave ps`.  For more information about addressing see (Address Allocation)[/site/features].


##Q: How do I expose one of my containers to the outside world?

How to expose a container to the outside world is described in [Exporting Services](/site/exporting-services).


##Q: How do I manually change the IP of a container?

My dockerized app checks the request of a remote IP, and I need to include a specific address on a list.  I could just use all of the IP addresses that Weave generates, but that's clearly not very efficient. 

You can manually change the IP of a container using [Classless Inter-Domain Routing or CIDR notation](https://en.wikipedia.org/wiki/Classless_Inter-Domain_Routing). For more information, refer to [How to Manually Change an IP Address](/site/manual-ip-address.md). 


##Q:How do I connect my existing 'legacy' network with a Weave container network?

For example, let's say you have a Weave network that runs on hosts A, B, C. And you have another host, let's call it P, where neither Weave nor Docker are running.  You want to connect from a process running on host P to a specific container running on your other network, Host B. 

Since the Weave Network is completely separate from any network that P is connected to, you cannot connect the container using the container's IP address.

Instead you will have to expose a port on the container on host B. You can read about how to expose a port: [http://docs.weave.works/weave/latest_release/features.html#service-export](XXXX-link to service export in Features doc)

>>Note: If host P is able to route to B, then this approach is the best solution.


##Q: Why am I seeing the same IP address assigned to two different containers on different hosts?

Under normal circumstances, this should not happen, but it can occur if  `weave forget` and `weave rmpeer` was run on more than one host. 

You cannot call `weave rmpeer` on more than one host. The address space, which was owned by the stale peer cannot be left dangling, and so, is reassigned. In this instance, the address is reassigned to the peer on which `weave rmpeer` was run. Therefore, if you run `weave forget` and then `weave rmpeer` on more than one host at a time, the result is duplicate IPs on more than one host.

Once the peers detect the inconsistency, they log the error and drop the connection that supplied the inconsistent data. The rest of the peers will carry on with their view of the world, but the network will not function correctly.

Some peers may be able to communicate their claim to the others before they run `rmpeer` (i.e. it's a race), so what you can expect is a few cliques of peers that are still talking to each other, but repeatedly dropping attempted connections with peers in other cliques.


##Q: What is the best practise for resetting a node that goes out of service?

When a node goes out of service, the best option is to call `weave rmpee` on one host and then `weave forget` on all the other hosts.

This will provide enough time for Weave to re-establish its peer connections across the mesh network. 


##Q: What about Weave's performance? Are software defined network overlays just as fast as native networking?

All virtualization techniques have some overhead, and unless your system is completely bottlenecked on the network you won't notice this during normal operation. 

Weave Net also automatically uses the fastest data path between two hosts. When Weave Net can't use the fast data path between two hosts, it falls back to the slower packet forwarding approach. Selecting the fastest forwarding approach is automatic, and is determined on a connection-by-connection basis. For example, a Weave network spanning two data centers might use fast data path within the data centers, but not for the more constrained network link between them.

For more information about fast datapath see [How Fast Data Path Works](/site/fastdp-how-it-works.md)


##Q:How can I tell if Weave is using fastdp or not?

To view whether Weave is using fastdp or not, you can run, `weave status connections`

For more information on this command, see [Viewing Connection Mode Fastdp or Sleeve](/site/fastdp/viewing-connections.md)


##Q: Does encryption work with fastdp?

Encryption does not work with fast datapath. If you enable encryption using the `--password` option to launch weave (or you use the `WEAVE_PASSWORD` environment variable), fast data path will by default be disabled. 

See [Using Fast Datapath](/site/using-fastdp.md) for more information

##Q: Can I have multiple isolated subnets and still enable individuals containers to communicate with one another?

Yes, of course!  Weave allows you to run isolated subnets and then open communications between individual containers from those isolated subnets. You can find information on how to do this in [Application Isolation](/site/application-isolation.md)
