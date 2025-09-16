---
title: FAQ
menu_order: 200
search_type: Documentation
---


<a name="container-ip"></a>
**Q: How do I obtain the IP of a specific container when I'm using Weave?**

You can use `weave ps <container>` to see the allocated address of a container on a Weave network.  

See [Troubleshooting Weave - List attached containers]({{ '/troubleshooting#list-attached-containers' | relative_url }}).


<a name="specific-ip"></a>
**Q: My dockerized app needs to check the request of an application that uses a static IP. Is it possible to manually change the IP of a container?**


You can manually change the IP of a container using [Classless Inter-Domain Routing or CIDR notation](https://en.wikipedia.org/wiki/Classless_Inter-Domain_Routing). 

For more information, refer to [Manually Specifying the IP Address of a Container]({{ '/tasks/ipam/manual-ip-address' | relative_url }}). 


<a name="expose-container"></a>
**Q: How do I expose one of my containers to the outside world?**

Exposing a container to the outside world is described in [Exporting Services]({{ '/tasks/manage/service-management#exporting' | relative_url }}).


<a name="legacy-network"></a>
**Q: Can I connect my existing 'legacy' network with a Weave container network?**

Yes you can. 

For example, you have a Weave network that runs on hosts A, B, C. and you have an additional host, that we'll call P, where neither Weave nor Docker are running.  However, you need to connect from a process running on host P to a specific container running on host B on the Weave network.  Since the Weave network is completely separate from any network that P is connected to, you cannot connect the container using the container's IP address. 

A simple way to accomplish this would be to run Weave on the host and then run, `weave expose` to expose the network to any running containers. Or you set up a route from P to one of A, B or C. See [Integrating a Network Host]({{ '/tasks/manage/host-network-integration' | relative_url }}).

Yet another option is to expose a port from the container on host B and then connect to it. You can read about exposing ports in [Exporting Services]({{ '/tasks/manage/service-management#exporting' | relative_url }}).


<a name="duplicate-peer"></a>
**Q: Why am I seeing "peer names collision" and failed connections?**

This sometimes happens when machines are cloned; we require each
machine in your cluster to have a unique identity.

For more information see [Peer Names]({{ '/operational-guide/concepts#peer-name' | relative_url }}).

Depending on your Linux distribution you may need to [set up
machine-id](https://www.freedesktop.org/software/systemd/man/machine-id.html)
or [generate a dbus id](https://dbus.freedesktop.org/doc/dbus-uuidgen.1.html).

<a name="duplicate-ip"></a>
**Q: Why am I seeing the same IP address assigned to two different containers on different hosts?**

Under normal circumstances, this should never happen, but it can occur if `weave rmpeer` was run on more than one host. 

For more information see [Starting, Stopping and Removing Peers]({{ '/tasks/ipam/stop-remove-peers-ipam' | relative_url }}).


<a name="dead-node"></a>
**Q: What is the best practice for resetting a node that goes out of service?**

When a node goes out of service, the best option is to call `weave rmpeer` on one host and then `weave forget` on all the other hosts.

See [Starting, Stopping and Removing Peers]({{ '/tasks/ipam/stop-remove-peers-ipam' | relative_url }}) for an in-depth discussion.


<a name="performance"></a>
**Q: What about Weave's performance? Are software defined network overlays just as fast as native networking?**

All virtualization techniques have some overhead, and Weave's overhead is typically around 2-3%. Unless your system is completely bottlenecked on the network, you won't notice this during normal operation. 

Weave Net also automatically uses the fastest datapath between two hosts. When Weave Net can't use the fast datapath between two hosts, it falls back to the slower packet forwarding approach. Selecting the fastest forwarding approach is automatic, and is determined on a connection-by-connection basis. For example, a Weave network spanning two data centers might use fast datapath within the data centers, but not for the more constrained network link between them.

For more information about fast datapath see [How Fast Datapath Works]({{ '/concepts/fastdp-how-it-works' | relative_url }}).


<a name="query-fastdp"></a>
**Q: How can I tell if Weave is using fast datapath (fastdp) or not?**

To view whether Weave is using fastdp or not, you can run, `weave status connections`

For more information on this command, see [Using Fast Datapath]({{ '/tasks/manage/fastdp' | relative_url }}).


<a name="encrypted-fastdp"></a>
**Q: Does encryption work with fastdp?**

Yes, 1.9 version of Weave Net added the encryption feature to fastdp.

See [Using Fast Datapath]({{ '/tasks/manage/fastdp' | relative_url }}) for more information.

<a name="app-isolation"></a>
**Q: Can I create multiple networks where containers can communicate on one network, but are isolated from containers on other networks?**

Yes, of course!  Weave allows you to run isolated networks and still allow open communications between individual containers from those isolated networks. You can find information on how to do this in [Application Isolation]({{ '/tasks/manage/application-isolation' | relative_url }}).


**<a name="ports"></a>Q: Which ports does Weave Net use (e.g. if I am configuring a firewall) ?**

You must permit traffic to flow through TCP 6783 and UDP 6783/6784,
which are Weave’s control and data ports.

The daemon also uses TCP 6781/6782 for
[metrics]({{ '/tasks/manage/metrics#metrics-endpoint-addresses' | relative_url }}), but
you would only need to open up this port if you wish to collect metrics
from another host.

The Weave Net daemon listens on localhost (127.0.0.1) TCP port 6784
for commands from other Weave Net components. This port should not be
opened to other hosts.

When using encrypted fast datapath, make sure that underlying
network does not block ESP traffic (IP protocol 50). For instance
on Google Cloud Platform a firewall rule for allowing ESP traffic has
to be installed.

~~**<a name="own-image"></a>Q: Why do you use your own Docker image `weaveworks/ubuntu`?**

The official Ubuntu image does not contain the `ping` and `nc`
commands which are used in many of our examples throughout the
documentation. The `weaveworks/ubuntu` image is simply the official
Ubuntu image with those two commands added.~~
We now use the official `alpine:latest` image, which already has
these two commands.


**See Also**

 * [Troubleshooting Weave]({{ '/troubleshooting' | relative_url }})
 * [Troubleshooting IPAM]({{ '/tasks/ipam/ipam' | relative_url }})
 * [Troubleshooting the Proxy]({{ '/tasks/weave-docker-api/using-proxy' | relative_url }})
 
