---
title: Integrating Docker via the Network Plugin (V2)
menu_order: 20
search_type: Documentation
---

 * [Installation](#installation)
 * [Configuring the Plugin](#configuring)
 * [Running Services or Containers Using the Plugin](#usage)

Docker Engine version 1.12 introduced [a new plugin system (V2)](https://docs.docker.com/engine/extend/).
This document describes how to use the Network Plugin V2 of Weave Net.

Before using the plugin, please keep in mind the plugin works only in Swarm
mode and requires Docker version 1.13 or later.

### <a name="installation"></a>Installation

To install the plugin run the following command on _each_ host already
participating in a Swarm cluster, i.e. on all master and worker nodes:

    $ docker plugin install rajchaudhuri/net-plugin:latest_release-<architecture>

Here, &lt;architecture> is the processor architecture on which your Docker Engine runs. 
It is a necessary part of the plugin name, because Docker currently does not support
multi-architecture plugins. The Weave Net Plugin V2 is available for the following 
architectures:

* amd64
* arm
* arm64
* ~~ppc64le~~
* ~~s390x~~

You can use the following commands to install the plugin appropriate to your architecture:

{% raw %}    $ ARCH=$(docker version -f '{{.Server.Arch}}'){% endraw %}
    $ docker plugin install rajchaudhuri/net-plugin:latest_release-$ARCH

Docker will pull the plugin from Docker Store, and it will ask to grant
privileges before installing the plugin. Afterwards, it will start `weaver`
process which will try to connect to Swarm masters running Weave Net.

### <a name="configuring"></a>Configuring the Plugin

There are several configuration parameters which can be set with:

    $ docker plugin set rajchaudhuri/net-plugin:latest_release-$ARCH PARAM=VALUE

The parameters include:

* `WEAVE_PASSWORD` - if non empty, it will instruct Weave Net to encrypt
   traffic - see [here]({{ '/tasks/manage/security-untrusted-networks' | relative_url }}) for
   more details.
* `WEAVE_MULTICAST` - set to 1 on each host running the plugin to enable
  multicast traffic on any Weave Net network.
* `WEAVE_MTU` - Weave Net defaults to 1376 bytes, but you can set a
  smaller size if your underlying network has a tighter limit, or set
  a larger size for better performance if your network supports jumbo
  frames - see [here]({{ '/tasks/manage/fastdp#mtu' | relative_url }}) for more
  details.
* `IPALLOC_RANGE` - the range of IP addresses used by Weave Net and the subnet
  they are placed in (CIDR format; default 10.32.0.0/12).
  See [here]({{ '/tasks/ipam/configuring-weave' | relative_url }}) for more details.

Before setting any parameter, the plugin has to be disabled with:

    $ docker plugin disable rajchaudhuri/net-plugin:latest_release-$ARCH

To re-enable the plugin run the following command:

    $ docker plugin enable rajchaudhuri/net-plugin:latest_release-$ARCH

### <a name="usage"></a>Running Services or Containers Using the Plugin

After you have launched the plugin, you can create a network for Docker Swarm
services by running the following command on any Docker Swarm master node:

    $ docker network create --driver=rajchaudhuri/net-plugin:latest_release-$ARCH mynetwork

Or you can create a network for any Docker container with:

    $ docker network create --driver=rajchaudhuri/net-plugin:latest_release-$ARCH --attachable mynetwork

To start a service attached to the network run, for example:

    $ docker service create --network=mynetwork ...

To ensure that name resolution (service discovery) works as expected, do one of the following:

* Include `--endpoint-mode=dnsrr` while creating a service. This will correctly resolve the service name to the load-balanced IP address of one of the task containers.
* OR, include `--hostname=SOMENAME` while creating a servce. This will also resolve `SOMENAME` to the load-balanced IP address of one of the task containers.

To start a container:

    $ docker run --network=mynetwork ...

This does not require a `--hostname` option. The container name and any network aliases will correctly resolve to the container's IP address.

**See Also**

 * [Integrating Docker via the Network Plugin (Legacy)]({{ '/install/plugin/plugin' | relative_url }})
 * [How the Weave Network Plugins Work]({{ '/install/plugin/plugin-how-it-works' | relative_url }})
 * [Troubleshooting the V2 plugin]({{ '/troubleshooting#v2plugin' | relative_url }})
