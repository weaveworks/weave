---
title: Integrating Docker via the Network Plugin (Legacy)
menu_order: 30
search_type: Documentation
---

 * [Launching Weave Net and Running Containers Using the Plugin](#launching)
 * [Restarting the Plugin](#restarting)

Docker versions 1.9 and later have a plugin mechanism for adding
different network providers. Weave Net installs itself as a network
plugin when you start it with `weave launch`. The Weave Docker
Networking Plugin is fast and easy to use, and, unlike other
networking plugins, does not require an external cluster store.

To create a network which can span multiple Docker hosts, Weave Net peers must be connected to each other, by specifying the other hosts during `weave launch` or via
[`weave connect`]({{ '/tasks/manage/finding-adding-hosts-dynamically' | relative_url }}).

See [Launching Weave Net]({{ '/install/using-weave#peer-connections' | relative_url }}) for a discussion on peer connections. 

After you've launched Weave Net and peered your hosts,  you can start containers using the following, for example:

    $ docker run --net=weave -ti weaveworks/ubuntu

on any of the hosts, and they can all communicate with each other
using any protocol, even multicast.

In order to use Weave Net's [Service Discovery]({{ '/tasks/weavedns/weavedns' | relative_url }}) you
must pass the additional arguments `--dns` and `-dns-search`, for
which a helper is provided in the Weave script:

    $ docker run --net=weave -h foo.weave.local $(weave dns-args) -tdi weaveworks/ubuntu
    $ docker run --net=weave -h bar.weave.local $(weave dns-args) -ti weaveworks/ubuntu
    # ping foo


### <a name="launching"></a>Launching Weave Net and Running Containers Using the Plugin

Just launch the Weave Net router onto each host and make a peer connection with the other hosts:

    host1$ weave launch host2
    host2$ weave launch host1

then run your containers using the Docker command-line:

    host1$ docker run --net=weave -ti weaveworks/ubuntu
    root@1458e848cd90:/# hostname -i
    10.32.0.2

    host2$ docker run --net=weave -ti weaveworks/ubuntu
    root@8cc4b5dc5722:/# ping 10.32.0.2

    PING 10.32.0.2 (10.32.0.2) 56(84) bytes of data.
    64 bytes from 10.32.0.2: icmp_seq=1 ttl=64 time=0.116 ms
    64 bytes from 10.32.0.2: icmp_seq=2 ttl=64 time=0.052 ms


### <a name="multi"></a>Creating multiple Docker Networks

Docker enables you to create multiple independent networks and attach
different sets of containers to each network. However, coordinating
this between hosts requires that you run Docker in ["swarm mode"](https://docs.docker.com/engine/swarm/swarm-mode/) or configure a
["key-value store"](https://docs.docker.com/engine/userguide/networking/get-started-overlay/#/set-up-a-key-value-store).

To operate in swarm mode, you are required to use the plugin v2 of Weave Net.
See [Integrating Docker via the Network Plugin (V2)]({{ '/install/plugin/plugin-v2' | relative_url }}) for
more details.

If your Docker installation has a key-value store, create a network
based on Weave Net as follows:

    $ docker network create --driver=weave mynetwork

then use it to connect a container:

    $ docker run --net=mynetwork ...

or

    $ docker network connect mynetwork somecontainer

Containers attached to different Docker Networks are
[isolated through subnets]({{ '/tasks/manage/application-isolation' | relative_url }}).


### <a name="restarting"></a>Restarting the Plugin

The plugin, like all Weave Net components, is started with a policy of `--restart=always`, so that it is always there after a restart or reboot. If you remove this container (for example, when using `weave reset`) before removing all endpoints created using `--net=weave`, Docker may hang for a long time when it subsequently tries to re-establish communications to the plugin.

Unfortunately, [Docker 1.9 may also try to communicate with the plugin before it has even started it](https://github.com/docker/libnetwork/issues/813).

If you are using `systemd` with Docker 1.9, it is advised that you modify the Docker unit to remove the timeout on startup. This gives Docker enough time to abandon its attempts. For example, in the file `/lib/systemd/system/docker.service`, add the following under `[Service]`:

    TimeoutStartSec=0


**See Also**

 * [How the Weave Network Plugins Work]({{ '/install/plugin/plugin-how-it-works' | relative_url }})
