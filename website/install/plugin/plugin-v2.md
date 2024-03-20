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

    $ docker plugin install weaveworks/net-plugin:latest_release

Docker will pull the plugin from Docker Store, and it will ask to grant
privileges before installing the plugin. Afterwards, it will start `weaver`
process which will try to connect to Swarm masters running Weave Net.

### <a name="configuring"></a>Configuring the Plugin

There are several configuration parameters which can be set with:

    $ docker plugin set weaveworks/net-plugin:latest_release PARAM=VALUE

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

    $ docker plugin disable weaveworks/net-plugin:latest_release

To re-enable the plugin run the following command:

    $ docker plugin enable weaveworks/net-plugin:latest_release

### <a name="usage"></a>Running Services or Containers Using the Plugin

After you have launched the plugin, you can create a network for Docker Swarm
services by running the following command on any Docker Swarm master node:

    $ docker network create --driver=weaveworks/net-plugin:latest_release mynetwork

Or you can create a network for any Docker container with:

    $ docker network create --driver=weaveworks/net-plugin:latest_release --attachable mynetwork

To start a service attached to the network run, for example:

    $ docker service create --network=mynetwork ...

Or to start a container:

    $ docker run --network=mynetwork ...

**See Also**

 * [Integrating Docker via the Network Plugin (Legacy)]({{ '/install/plugin/plugin' | relative_url }})
 * [How the Weave Network Plugins Work]({{ '/install/plugin/plugin-how-it-works' | relative_url }})
 * [Troubleshooting the V2 plugin]({{ '/troubleshooting#v2plugin' | relative_url }})
