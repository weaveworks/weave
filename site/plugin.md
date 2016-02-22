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

> NB: It is inadvisable to attach containers to the weave network
> using both the Weave Docker Networking Plugin and
> [Weave Docker API Proxy](proxy.html) simultaneously. Such containers
> will end up with two weave network interfaces and two IP addresses,
> which is rarely desirable. To ensure the proxy is not being used, *do
> not run `eval $(weave env)`, or `docker $(weave config) ...`*.

In order to use Weave's [Service Discovery](weavedns.html), you
need to pass the additional arguments `--dns` and `-dns-search`, for
which we provide a helper in the weave script:

    $ docker run --net=weave -h foo.weave.local $(weave dns-args) -tdi ubuntu

Here is a complete example of using the plugin for connectivity
between two containers running on different hosts:

    host1$ weave launch
    host1$ docker run --net=weave -h foo.weave.local $(weave dns-args) -tdi ubuntu
    
    host2$ weave launch $HOST1
    host2$ docker run --net=weave $(weave dns-args) -ti ubuntu
    root@cb73d1a8aece:/# ping -c1 -q foo
    PING foo (10.32.0.1) 56(84) bytes of data.
    
    --- foo ping statistics ---
    1 packets transmitted, 1 received, 0% packet loss, time 0ms
    rtt min/avg/max/mdev = 1.341/1.341/1.341/0.000 ms

## Under the hood

The Weave plugin actually provides *two* network drivers to Docker
- one named `weavemesh` that can operate without a cluster store and
one named `weave` that can only work with one (like Docker's overlay
driver).

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
store, but the first part of
[Getting Started with Docker Multi-host Networking](https://github.com/docker/docker/blob/master/docs/userguide/networking/get-started-overlay.md)
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
`--net=weave`, Docker may hang for a long time when it subsequently
tries to talk to the plugin.

Unfortunately, [Docker 1.9 may also try to talk to the plugin before it has even started it](https://github.com/docker/libnetwork/issues/813).
If using `systemd`, we advise that you modify the Docker unit to
remove the timeout on startup, to give Docker enough time to abandon
its attempts.

E.g. in the file `/lib/systemd/system/docker.service`, add under `[Service]`:

    TimeoutStartSec=0
