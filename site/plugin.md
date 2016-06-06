---
title: Integrating Docker via the Network Plugin
menu_order: 60
---

 * [Launching Weave Net and Running Containers Using the Plugin](#launching)
 * [Restarting the Plugin](#restarting)
 * [Bypassing the Central Cluster Store When Building Docker Apps](#cluster-store)
 * [Using other plugin command-line arguments](#plugin-args)

Docker versions 1.9 and later have a plugin mechanism for adding
different network providers. Weave Net installs itself as a network plugin
when you start it with `weave launch`. The Weave Docker Networking plugin is fast and easy to use and 
best of all doesn't require an external cluster store in order to use it.  

To create a network which can span multiple Docker hosts, Weave Net peers must be connected to each other, by specifying the other hosts during `weave launch` or via
[`weave connect`](/site/using-weave/finding-adding-hosts-dynamically.md).

See [Using Weave Net](/site/using-weave.md#peer-connections) for a discussion on peer connections. 

After you've launched Weave Net and peered your hosts,  you can start containers using the following, for example:

    $ docker run --net=weave -ti ubuntu

on any of the hosts, and they can all communicate with each other
using any protocol, even multicast.

>**Warning!** It is inadvisable to attach containers to the Weave network using the Weave Docker Networking Plugin and Weave Docker API Proxy simultaneously. Such containers will end up with two Weave network interfaces and two IP addresses, which is rarely desirable. To ensure that the proxy is not being used, do not run eval $(weave env), or docker $(weave config).

In order to use Weave Net's [Service Discovery](/site/weavedns.md) you
must pass the additional arguments `--dns` and `-dns-search`, for
which a helper is provided in the Weave script:

    $ docker run --net=weave -h foo.weave.local $(weave dns-args) -tdi ubuntu
    $ docker run --net=weave -h bar.weave.local $(weave dns-args) -ti ubuntu
    # ping foo


###<a name="launching"></a>Launching Weave Net and Running Containers Using the Plugin

Just launch the Weave Net router onto each host and make a peer connection with the other hosts:

    host1$ weave launch host2
    host2$ weave launch host1

then run your containers using the Docker command-line:

    host1$ docker run --net=weave -ti ubuntu
    root@1458e848cd90:/# hostname -i
    10.32.0.2

    host2$ docker run --net=weave -ti ubuntu
    root@8cc4b5dc5722:/# ping 10.32.0.2

    PING 10.32.0.2 (10.32.0.2) 56(84) bytes of data.
    64 bytes from 10.32.0.2: icmp_seq=1 ttl=64 time=0.116 ms
    64 bytes from 10.32.0.2: icmp_seq=2 ttl=64 time=0.052 ms


###<a name="restarting"></a>Restarting the Plugin

The plugin, like all Weave Net components, is started with a policy of `--restart=always`, so that it is always there after a restart or reboot. If you remove this container (for example, when using `weave reset`) before removing all endpoints created using `--net=weave`, Docker may hang for a long time when it subsequently tries to re-establish communications to the plugin.

Unfortunately, [Docker 1.9 may also try to communicate with the plugin before it has even started it](https://github.com/docker/libnetwork/issues/813).

If you are using `systemd` with Docker 1.9, it is advised that you modify the Docker unit to remove the timeout on startup. This gives Docker enough time to abandon its attempts. For example, in the file `/lib/systemd/system/docker.service`, add the following under `[Service]`:

    TimeoutStartSec=0

###<a name="cluster-store"></a>Bypassing the Central Cluster Store When Building Docker Apps

To run a Docker cluster without a central database, you need to ensure the following:

 1. Run in "local" scope. This tells Docker to ignore any cross-host coordination.
 2. Allow Weave to handle all the cross-host coordination and to set up all networks. This is done by using the `weave launch` command.
 3. Provide an IP Address Management (IPAM) driver, which links to Weave Net's own IPAM system

All cross-host coordination is handled by Weave Net's "mesh" communication, using gossipDNS and eventual consistency to avoid the need for constant communication and dependency on a central cluster store.


###<a name="plugin-args"></a>Using other plugin command-line arguments

If you need to give additional arguments to the plugin independently, don't
use `weave launch`, but instead run:

    $ weave launch-router [other peers]
    $ weave launch-plugin [plugin arguments]

The plugin command-line arguments are:

 * `--log-level=debug|info|warning|error` --tells the plugin
   how much information to emit for debugging.
 * `--no-restart` -- remove the default policy of `--restart=always`, if
   you want to control start-up of the plugin yourself


>Note: When using the Docker Plugin, there is no need to run `eval
 $(weave env)` to enable the Proxy. If you do, you may end up with two
 weave network interfaces and two IP addresses for each container.

**See Also**

 * [How the Weave Network Plugin Works](/site/plugin/plugin-how-it-works.md)
 * [Plugin Command-line Arguments](/site/plugin/plug-in-command-line.md)
 

