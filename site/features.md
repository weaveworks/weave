---
title: Weave Features
layout: default
---

# Weave Features

Weave has a few more features beyond those illustrated by the [basic
example](https://github.com/weaveworks/weave#example):

 * [Virtual ethernet switch](#virtual-ethernet-switch)
 * [Seamless Docker integration](#docker)
 * [Address allocation](#addressing)
 * [Naming and discovery](#naming-and-discovery)
 * [Application isolation](#application-isolation)
 * [Dynamic network attachment](#dynamic-network-attachment)
 * [Security](#security)
 * [Host network integration](#host-network-integration)
 * [Service export](#service-export)
 * [Service import](#service-import)
 * [Service binding](#service-binding)
 * [Service routing](#service-routing)
 * [Multi-cloud networking](#multi-cloud-networking)
 * [Multi-hop routing](#multi-hop-routing)
 * [Dynamic topologies](#dynamic-topologies)
 * [Container mobility](#container-mobility)
 * [Fault tolerance](#fault-tolerance)

### <a name="virtual-ethernet-switch"></a>Virtual Ethernet Switch

To application containers, the network established by weave looks
like a giant Ethernet switch to which all the containers are
connected.

Containers can easily access services from each other; e.g. in the
container on `$HOST1` we can start a netcat "service" with

    root@a1:/# nc -lk -p 4422

and then connect to it from the container on `$HOST2` with

    root@a2:/# echo 'Hello, world.' | nc a1 4422

Note that *any* protocol is supported. Doesn't even have to be over
TCP/IP, e.g. a netcat UDP service would be run with

    root@a1:/# nc -lu -p 5533

    root@a2:/# echo 'Hello, world.' | nc -u a1 5533

We can deploy the entire arsenal of standard network tools and
applications, developed over decades, to configure, secure, monitor,
and troubleshoot our container network. To put it another way, we can
now re-use the same tools and techniques when deploying applications
as containers as we would have done when deploying them 'on metal' in
our data centre.

### <a name="docker"></a>Seamless Docker integration

Weave includes a [proxy](proxy.html) so that containers launched via
the Docker [command-line interface](https://docs.docker.com/reference/commandline/cli/) or
[remote API](https://docs.docker.com/reference/api/docker_remote_api/)
are attached to the weave network before they begin execution.

### <a name="addressing"></a>Address allocation

Containers are automatically allocated an IP address that is unique
across the weave network. You can see which address was allocated with
[`weave ps`](troubleshooting.html#list-attached-containers):

    host1$ weave ps a1
    a7aee7233393 7a:44:d3:11:10:70 10.128.0.2/10

Weave detects when a container has exited and releases its
automatically allocated addresses so they can be re-used.

See the [Automatic IP Address Management](ipam.html) documentation for
further details.

Instead of getting weave to allocate IP addresses automatically, it is
also possible to specify an address and network explicitly, expressed
in
[CIDR notation](http://en.wikipedia.org/wiki/Classless_Inter-Domain_Routing#CIDR_notation)
- let's see how the first example would have looked:

On $HOST1:

    host1$ docker run -e WEAVE_CIDR=10.2.1.1/24 -ti ubuntu
    root@7ca0f6ecf59f:/#

And $HOST2:

    host2$ docker run -e WEAVE_CIDR=10.2.1.2/24 -ti ubuntu
    root@04c4831fafd3:/#

Then in the container on $HOST1...

    root@7ca0f6ecf59f:/# ping -c 1 -q 10.2.1.2
    PING 10.2.1.2 (10.2.1.2): 48 data bytes
    --- 10.2.1.2 ping statistics ---
    1 packets transmitted, 1 packets received, 0% packet loss
    round-trip min/avg/max/stddev = 1.048/1.048/1.048/0.000 ms

Similarly, in the container on $HOST2...

    root@04c4831fafd3:/# ping -c 1 -q 10.2.1.1
    PING 10.2.1.1 (10.2.1.1): 48 data bytes
    --- 10.2.1.1 ping statistics ---
    1 packets transmitted, 1 packets received, 0% packet loss
    round-trip min/avg/max/stddev = 1.034/1.034/1.034/0.000 ms

The IP addresses and netmasks can be anything you like, but make sure
they don't conflict with any IP ranges in use on the hosts (including
those delegated to weave's [automatic IP address allocator](ipam.html)) or
IP addresses of external services the hosts or containers need to
connect to. The same IP range must be used everywhere, and the
individual IP addresses must, of course, be unique.

### <a name="naming-and-discovery"></a>Naming and discovery

Named containers are automatically registered in
[weaveDNS](weavedns.html), which makes them discoverable through
simple name lookups:

    host1$ docker run -dti --name=service ubuntu
    host1$ docker run -ti ubuntu
    root@7b21498fb103:/# ping service

This feature supports load balancing, fault resilience and hot
swapping; see the [weaveDNS](weavedns.html) documentation for more
details.

### <a name="application-isolation"></a>Application isolation

A single weave network can host multiple, isolated applications, with
each application's containers being able to communicate with each
other but not containers of other applications.

To accomplish that, we assign each application a different subnet.
Let's begin by configuring weave's allocator to manage multiple
subnets:

    host1$ weave launch -iprange 10.2.0.0/16 -ipsubnet 10.2.1.0/24
    host1$ eval $(weave proxy-env)
    host2$ weave launch -iprange 10.2.0.0/16 -ipsubnet 10.2.1.0/24 $HOST1
    host2$ eval $(weave proxy-env)

This delegates the entire 10.2.0.0/16 subnet to weave, and instructs
it to allocate from 10.2.1.0/24 within that if no specific subnet is
specified. Now we can launch some containers in the default subnet:

    host1$ docker run --name a1 -ti ubuntu
    host2$ docker run --name a2 -ti ubuntu

And some more containers in a different subnet:

    host1$ docker run -e WEAVE_CIDR=net:10.2.2.0/24 --name b1 -ti ubuntu
    host2$ docker run -e WEAVE_CIDR=net:10.2.2.0.24 --name b2 -ti ubuntu

A quick 'ping' test in the containers confirms that they can talk to
each other but not the containers of our first application...

    root@b1:/# ping -c 1 -q b2
    PING b2.weave.local (10.2.2.128) 56(84) bytes of data.
    --- b2.weave.local ping statistics ---
    1 packets transmitted, 1 received, 0% packet loss, time 0ms
    rtt min/avg/max/mdev = 1.338/1.338/1.338/0.000 ms

    root@b1:/# ping -c 1 -q a1
    PING a1.weave.local (10.2.1.2) 56(84) bytes of data.
    --- a1.weave.local ping statistics ---
    1 packets transmitted, 0 received, 100% packet loss, time 0ms

    root@b1:/# ping -c 1 -q a2
    PING a2.weave.local (10.2.1.130) 56(84) bytes of data.
    --- a2.weave.local ping statistics ---
    1 packets transmitted, 0 received, 100% packet loss, time 0ms


This isolation-through-subnets scheme is an example of carrying over a
well-known technique from the 'on metal' days to containers.

If desired, a container can be attached to multiple subnets when it is
started:

    host1$ docker run -e WEAVE_CIDR="net:default net:10.2.2.0/24" -ti ubuntu

`net:default` is used here to request allocation of an address from
the default subnet in addition to one from an explicitly specified
range.

NB: By default docker permits communication between containers on the
same host, via their docker-assigned IP addresses. For complete
isolation between application containers, that feature needs to be
disabled by
[setting `--icc=false`](https://docs.docker.com/articles/networking/#between-containers)
in the docker daemon configuration. Furthermore, containers should be
prevented from capturing and injecting raw network packets - this can
be accomplished by starting them with the `--cap-drop net_raw` option.

### <a name="dynamic-network-attachment"></a>Dynamic network attachment

Sometimes the application network to which a container should be
attached is not known in advance. For these situations, weave allows
an existing, running container to be attached to the weave network. To
illustrate, we can achieve the same effect as the first example with

    host1$ C=$(docker run -e WEAVE_CIDR=none -dti ubuntu)
    host1$ weave attach $C
    10.2.1.3

(Note that since we modified `DOCKER_HOST` to point to the proxy
earlier, we have to pass `-e WEAVE_CIDR=none` to start a container
that _doesn't_ get automatically attached to the weave network for the
purposes of this example.)

The output shows the IP address that got allocated, in this case on
the default subnet.

There is a matching `weave detach` command:

    host1$ weave detach $C
    10.2.1.3

You can detach a container from one application network and attach it
to another:

    host1$ weave detach net:default $C
    10.2.1.3
    host1$ weave attach net:10.2.2.0/24 $C
    10.2.2.3

or attach a container to multiple application networks, effectively
sharing it between applications:

    host1$ weave attach net:default
    10.2.1.3
    host1$ weave attach net:10.2.2.0/24
    10.2.2.3

Finally, multiple addresses can be attached or detached with a single
invocation:

    host1$ weave attach net:default net:10.2.2.0/24 net:10.2.3.0/24 $C
    10.2.1.3 10.2.2.3 10.2.3.1
    host1$ weave detach net:default net:10.2.2.0/24 net:10.2.3.0/24 $C
    10.2.1.3 10.2.2.3 10.2.3.1

### <a name="security"></a>Security

In order to connect containers across untrusted networks, weave peers
can be told to encrypt traffic by supplying a `-password` option or
`WEAVE_PASSWORD` environment variable when launching weave, e.g.

    host1$ weave launch -password wEaVe

or

    host1$ export WEAVE_PASSWORD=wEaVe
    host1$ weave launch

_NOTE: The command line option takes precedence over the environment
variable._

The password needs to be reasonably strong to guard against online
dictionary attacks. We recommend at least 50 bits of entropy. An easy
way to generate a random password which satsifies this requirement is

    < /dev/urandom tr -dc A-Za-z0-9 | head -c9 ; echo

The same password must be specified for all weave peers.

### <a name="host-network-integration"></a>Host network integration

Weave application networks can be integrated with a host's network,
establishing connectivity between the host and application containers
anywhere.

Let's say that in our example we want `$HOST2` to have access to the
application containers. On `$HOST2` we run

    host2$ weave expose
    10.2.1.132

This grants the host access to all application containers in the
default subnet. An IP address is allocated for that purpose, which is
returned. So now

    host2$ ping 10.2.1.132

will work, and, more interestingly, we can ping our `a1` application
container, which is residing on `$HOST1`, after finding its IP
address:

    host1$ weave ps a1
    a1 1e:88:d7:5b:77:68 10.2.1.2/24

    host2$ ping 10.2.1.2

Multiple subnet addresses can be exposed or hidden with a single
invocation:

    host2$ weave expose net:default net:10.2.2.0/24
    10.2.1.132 10.2.2.130
    host2$ weave hide   net:default net:10.2.2.0/24
    10.2.1.132 10.2.2.130

Finally, exposed addresses can be added to weaveDNS by supplying a
fully-qualified domain name:

    host2$ weave expose -h exposed.weave.local
    10.2.1.132

### <a name="service-export"></a>Service export

Services running in containers on a weave network can be made
accessible to the outside world (and, more generally, other networks)
from any weave host, irrespective of where the service containers are
located.

Say we want to make our example netcat "service", which is running in
a container on `$HOST1`, accessible to the outside world via `$HOST2`.

First we need to expose the application network to `$HOST2`, as
explained [above](#host-network-integration), i.e.

    host2$ weave expose
    10.2.1.132

Then we add a NAT rule to route from the outside world to the
destination container service.

    host1$ weave ps a1
    a1 1e:88:d7:5b:77:68 10.2.1.2/24

    host2$ iptables -t nat -A PREROUTING -p tcp -i eth0 --dport 2211 \
           -j DNAT --to-destination 10.2.1.2:4422

Here we are assuming that the "outside world" is connecting to `$HOST2`
via 'eth0'. We want TCP traffic to port 2211 on the external IPs to be
routed to our 'nc' service, which is running on port 4422 in the
container a1.

With the above in place, we can connect to our 'nc' service from
anywhere with

    echo 'Hello, world.' | nc $HOST2 2211

(NB: due to the way routing is handled in the Linux kernel, this won't
work when run *on* `$HOST2`.)

Similar NAT rules to the above can used to expose services not just to
the outside world but also other, internal, networks.

### <a name="service-import"></a>Service import

Applications running in containers on a weave network can be given
access to services which are only reachable from certain weave hosts,
irrespective of where the application containers are located.

Say that, as an extension of our example, we have a netcat service
running on `$HOST3`, port 2211, and that `$HOST3` is not part of the weave
network and is only reachable from `$HOST1`, not `$HOST2`. Nevertheless we
want to make the service accessible to an application running in a
container on `$HOST2`.

First we need to expose the application network to the host, as
explained [above](#host-network-integration), this time on `$HOST1`,
i.e.

    host1$ weave expose -h host1.weave.local
    10.2.1.3

Then we add a NAT rule to route from the above IP to the destination
service.

    host1$ iptables -t nat -A PREROUTING -p tcp -d 10.2.1.3 --dport 3322 \
           -j DNAT --to-destination $HOST3:2211

This allows any application container to reach the service by
connecting to 10.2.1.3:3322. So if `$HOST3` is indeed running a netcat
service on port 2211, e.g.

    host3$ nc -lk -p 2211

then we can connect to it from our application container on `$HOST2` with

    root@a2:/# echo 'Hello, world.' | nc host1 3322

The same command will work from any application container.

### <a name="service-binding"></a>Service binding

Importing a service provides a degree of indirection that allows late
and dynamic binding, similar to what can be achieved with a proxy. In
our example, application containers are unaware that the service they
are accessing at `10.2.1.3:3322` is in fact residing on
`$HOST3:2211`. We can point application containers at another service
location by changing the above NAT rule, without having to alter the
applications themselves.

### <a name="service-routing"></a>Service routing

The [service export](#service-export) and
[service import](#service-import) features can be combined to
establish connectivity between applications and services residing on
disjoint networks, even when those networks are separated by firewalls
and might have overlapping IP ranges. Each network imports its
services into weave, and in turn exports from weave services required
by its applications. There are no application containers in this
scenario (though of course there could be); weave is acting purely as
an address translation and routing facility, using the weave
application network as an intermediary.

In our example above, the netcat service on `$HOST3` is imported into
weave via `$HOST1`. We can export it on `$HOST2` by first exposing the
application network with

    host2$ weave expose
    10.2.1.3

and then adding a NAT rule which routes traffic from the `$HOST2`
network (i.e. anything which can connect to `$HOST2`) to the service
endpoint in the weave network

    host2$ iptables -t nat -A PREROUTING -p tcp -i eth0 --dport 4433 \
           -j DNAT --to-destination 10.2.1.3:3322

Now any host on the same network as `$HOST2` can access the service with

    echo 'Hello, world.' | nc $HOST2 4433

Furthermore, as explained in [service-binding](#service-binding), we
can dynamically alter the service locations without having to touch
the applications that access them, e.g. we could move the example
netcat service to `$HOST4:2211` while retaining its 10.2.1.3:3322
endpoint in the weave network.

### <a name="multi-cloud-networking"></a>Multi-cloud networking

Weave can network containers hosted in different cloud providers /
data centres. So, for example, one could run an application consisting
of containers on GCE, EC2 and in local data centres.

To enable this, the network must be configured to permit TCP and UDP
connections to the weave port of the docker hosts. The weave port
defaults to 6783. This can be overriden by setting `WEAVE_PORT`, but
it is highly recommended that all peers in a weave network are given
the same port setting.

### <a name="multi-hop-routing"></a>Multi-hop routing

A network of containers across more than two hosts can be established
even when there is only partial connectivity between the hosts. Weave
is able to route traffic between containers as long as there is at
least one *path* of connected hosts between them.

So, for example, if a docker host in a local data centre can connect
to hosts in GCE and EC2, but the latter two cannot connect to each
other, containers in the latter two can still communicate; weave will
route the traffic via the local data centre.

### <a name="dynamic-topologies"></a>Dynamic topologies

To add a host to an existing weave network, one simply launches weave
on the host, supplying the address of at least one existing
host. Weave will automatically discover the other hosts in the network
and establish connections to them if it can (in order to avoid
unnecessary multi-hop routing).

In some situations all existing weave hosts may be unreachable from
the new host due to firewalls, etc. However, it is still possible to
add the new host, provided inverse connections, i.e. from existing
hosts to the new hosts, are possible. To accomplish that, one launches
weave on the new host without supplying any additional addresses, and
then on one of the existing hosts runs

    host# weave connect $NEW_HOST

Other hosts in the weave network will automatically attempt to
establish connections to the new host too. Conversely, it is possible
to instruct a peer to forget a particular host that was specified
to it via `weave launch` or `weave connect`:

    host# weave forget $DECOMMISSIONED_HOST

This will prevent the peer from trying to reconnect to that host once
connectivity to it is lost, and thus can be used to administratively
remove decommissioned peers from the network.

Hosts can also be bulk-replaced. All existing hosts will be forgotten,
and the new hosts will be added, when one runs

    host# weave connect --replace $NEW_HOST1 $NEW_HOST2

For complete control over the peer topology, automatic discovery can
be disabled with the `-nodiscovery` option to `weave launch`. In this
mode, weave will only connect to the addresses specified at launch
time and with `weave connect`.

### <a name="container-mobility"></a>Container mobility

Containers can be moved between hosts without requiring any
reconfiguration or, in many cases, restarts of other containers. All
that is required is for the migrated container to be started with the
same IP address as it was given originally.

### <a name="fault-tolerance"></a>Fault tolerance

Weave peers continually exchange topology information, and monitor
and (re)establish network connections to other peers. So if hosts or
networks fail, weave can "route around" the problem. This includes
network partitions; containers on either side of a partition can
continue to communicate, with full connectivity being restored when
the partition heals.

The weave container is very light-weight - just over 8MB image size
and a few 10s of MBs of runtime memory - and disposable. I.e. should
weave ever run into difficulty, one can simply stop it (with `weave
stop`) and restart it. Application containers do *not* have to be
restarted in that event, and indeed may not even experience a
temporary connectivity failure if the weave container is restarted
quickly enough.
