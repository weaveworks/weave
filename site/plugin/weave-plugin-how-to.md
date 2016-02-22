---
title: Using the Weave Net Docker Network Plugin
layout: default
---


Docker versions 1.9 and later have a plugin mechanism for adding
different network providers. Weave installs itself as a network plugin
when you start it with `weave launch`. To create a network that spans
multiple docker hosts, the weave peers must be connected in the usual
way, i.e. by specifying other hosts in `weave launch` or via
[`weave connect`](features.html#dynamic-topologies).

Subsequently you can start containers with, e.g.

    $ docker run --net=weave -ti ubuntu

on any of the hosts, and they can all communicate with each other.

In order to use Weave's [Service Discovery](weavedns.html), you
need to pass the additional arguments `--dns` and `-dns-search`, for
which we provide a helper in the weave script:

    $ docker run --net=weave -h foo.weave.local $(weave dns-args) -tdi ubuntu
    $ docker run --net=weave -h bar.weave.local $(weave dns-args) -ti ubuntu
    # ping foo


The Weave Docker Networking plugin is fast and easy to use and best of all doesn't require an external cluster store in order to use it.  

Just launch the Weave Net router onto each host and then make a peer connection with the other hosts:

~~~bash
host1$ weave launch host2
host2$ weave launch host1
~~~

then you can run your containers using the Docker command-line:

~~~bash
host1$ docker run --net=weave -ti ubuntu
root@1458e848cd90:/# hostname -i
10.32.0.2
~~~

~~~bash
host2$ docker run --net=weave -ti ubuntu
root@8cc4b5dc5722:/# ping 10.32.0.2

PING 10.32.0.2 (10.32.0.2) 56(84) bytes of data.
64 bytes from 10.32.0.2: icmp_seq=1 ttl=64 time=0.116 ms
64 bytes from 10.32.0.2: icmp_seq=2 ttl=64 time=0.052 ms
~~~

>>Note: When using the Docker Plugin, there is no need to run eval $(weave env) to enable the Proxy. Because Weave is running as a plugin within Docker, the Weave Docker API Proxy, at present, cannot detect between networks.  

### Restarting the Plugin

We start the plugin with a policy of `--restart=always`, so that it is there after a restart or reboot. If you remove this container (e.g. using `weave reset`) before removing all endpoints created using `--net=weave`, Docker may hang for a long time when it subsequently tries to talk to the plugin.

Unfortunately, [Docker 1.9 may also try to commmuncate with the plugin before it has even started it](https://github.com/docker/libnetwork/issues/813).

If using `systemd`, it's advised that you modify the Docker unit to remove the timeout on startup. This gives Docker enough time to abandon its attempts. For example, in the file `/lib/systemd/system/docker.service`, add the following under `[Service]`: 

~~~bash
    TimeoutStartSec=0
~~~

###Bypassing the Central Cluster Store When Building Docker Apps

To run a Docker cluster without a central database, you need to ensure the following:

 1. Run in "local" scope. This tells Docker to ignore any cross-host coordination.
 2. Allow Weave to handle all the cross-host coordination and to set up all networks. This is done by using the `weave launch` command.
 3. Provide an IP Address Management (IPAM) driver, which links to Weave Net's own IPAM system

All cross-host coordination is handled by Weave Net's "mesh" communication, using gossipDNS and eventual consistency to avoid the need for constant communication and dependency on a central cluster store.


