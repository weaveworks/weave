---
title: Using Weave Net via Docker Networking
layout: default
---

# Weave Plugin

Docker versions 1.9 and later have a plugin mechanism for adding
different network providers. Weave installs itself as a network plugin
when you start it with `weave launch`. To create a network that spans
multiple docker hosts, the weave peers must be connected in the usual
way, i.e. by specifying other hosts in `weave launch` or
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

## Under the hood

The Weave plugin actually provides *two* network drivers to Docker
- one named `weavemesh` that can operate without a cluster store and
one named `weave` that can only work with one (like Docker's overlay
driver).

Docker supports creating multiple networks via the plugin, although
Weave Net provides only one network underneath. So long as the IP
addresses for each network are on different subnets, containers on one
network will not be able to communicate with those on a different network.

### `weavemesh` driver

* Weave handles all co-ordination between hosts (referred to by Docker as a "local scope" driver)
* Uses Weave's partition tolerant IPAM
* User must pick different subnets if creating multiple networks.

We create a network named `weave` for you automatically, using the
[default subnet](ipam.html#subnets) set for the `weave` router.

To create additional networks using the `weavemesh` driver, pick a
different subnet and run this command on all hosts:

    $ docker network create --driver=weavemesh --ipam-driver=weavemesh --subnet=<subnet> <network-name>

The subnets you pick must be within the range covered by Weave's [IP
Address Management](ipam.html#range)

### `weave` driver

* This runs in what Docker call "global scope"; requires a cluster store
* Used with Docker's cluster store based IPAM
* Docker can coordinate choosing different subnets for multiple networks.

To create a network using this driver, once you have your cluster store set up:

    $ docker network create --driver=weave <network-name>

### <a name="cluster-store"></a>Cluster store

This is the term used by Docker for a distributed key-value store such
as Consul, Etcd or ZooKeeper.

There's no specific documentation from Docker on using a cluster
store, but the first part of
[Getting Started with Docker Multi-host Networking](https://github.com/docker/docker/blob/master/docs/userguide/networking/get-started-overlay.md)
should point the way.

## Plugin command-line arguments

If you want to give some arguments to the plugin independently, don't
use `weave launch`; instead run:

    $ weave launch-router [other peers]
    $ weave launch-plugin [plugin arguments]

The plugin command-line arguments are:

 * `--log-level=debug|info|warning|error`, which tells the plugin
   how much information to emit for debugging.
 * `--no-multicast-route`: stop weave adding a static IP route for
   multicast traffic on its interface

By default, multicast traffic will be routed over the weave network.
To turn this off, e.g. because you want to configure your own multicast
route, add the `--no-multicast-route` flag to `weave launch-plugin`.

## Restarting

We start the plugin with a policy of `--restart=always`, so that it is
there after a restart or reboot. If you remove this container
(e.g. using `weave reset`) before removing all endpoints created using
`--net=weave`, Docker may hang for a long time when it subsequently
tries to talk to the plugin.

Unfortunately, [Docker 1.9 may also try to talk to the plugin before it has even started it](https://github.com/docker/libnetwork/issues/813).
If using `systemd`, we advise that you modify the Docker unit to
remove the timeout on startup, to give Docker enough time to abandon
its attempts.

E.g. in the file `/lib/systemd/system/docker.service`, add under `[Service]`:

    TimeoutStartSec=0
