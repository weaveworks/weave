---
title: Using Weave Net via Docker Networking
layout: default
---

# Weave Plugin

New in Docker version 1.9 is a plugin mechanism to add different
network providers.

To install, download the `weave` script as per the [readme][].

The Weave Net plugin runs automatically when you `weave launch`.  You
must still tell the weave peers to connect to one another, either via
`weave launch` or `weave connect`.

## Starting a container

    $ docker run --net=weave -ti ubuntu

## Using WeaveDNS for Service Discovery

You need to pass the additional arguments `--dns` and `-dns-search`,
for which we provide a helper in the weave script:

    $ docker run --net=weave -h foo.weave.local $(weave dns-args) -tdi ubuntu
    $ docker run --net=weave -h bar.weave.local $(weave dns-args) -ti ubuntu
    # ping foo

For more details see the [Weave Service Discovery documentation][service-discovery].

## Under the hood

The Weave Net plugin actually provides *two* network drivers to Docker
- one named `weavemesh` that can operate without a [cluster
store](#cluster-store) (like Docker's overlay driver) and one named
`weave` that can only work with one.

### `weavemesh` driver

* Weave handles all co-ordination between hosts (referred to by Docker as a "local scope" driver)
* Supports only a single network (we create one named `weave` for you automatically)
* Uses Weave's partition tolerant IPAM

If you do create additional networks using the `weavemesh` driver,
containers attached to them will be able to communicate with
containers attached to `weave`; there is no isolation between those
networks.

### `weave` driver

* This runs in what Docker call "global scope"; requires a cluster store
* Supports multiple networks (these must be created with `docker network create --driver weave ...`)
* Used with Docker's cluster store based IPAM

There's no specific documentation from Docker on using a cluster
store, but the first part of [Getting Started with Docker Multi-host Networking][docker-net]
should point the way.

Note that in the case of multiple networks using the `weave` driver, all containers are
on the same virtual network but Docker allocates their addresses on
different subnets so they cannot talk to each other directly.

## Plugin command-line arguments

If you want to give some arguments to the plugin independently, don't
use `weave launch`; instead run:

    $ weave launch-router [other peers]
    $ weave launch-plugin [plugin arguments]

The plugin command-line arguments are:

 * `--log-level=debug|info|warning|error`, which tells the plugin
   how much information to emit for debugging.
 * `--mesh-network-name=<name>`: set it to blank to disable creation
   of a default network, or a name of your own choice.
 * `--no-multicast-route`: stop weave adding a static IP route for
   multicast traffic on its interface

By default, multicast traffic will be routed over the weave network.
To turn this off, e.g. because you want to configure your own multicast
route, add the `--no-multicast-route` flag to `weave launch-plugin`.

## Restarting

We start the plugin with a policy of `--restart=always`, so that it is
there after a restart or reboot. If you remove this container
(e.g. using `weave reset`) before removing all endpoints created using
`--net=weave`, Docker can
[hang](https://github.com/docker/libnetwork/issues/813).

[readme]: https://github.com/weaveworks/weave/blob/master/README.md#installation
[service-discovery]: weavedns.html
[docker-net]: https://github.com/docker/docker/blob/master/docs/userguide/networking/get-started-overlay.md
