---
title: Weave features
layout: default
---

# Weave features

Weave has a few more features beyond those illustrated by the [basic
example](https://github.com/weaveworks/weave#example):

 * [Virtual ethernet switch](#virtual-ethernet-switch)
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
 * [Automatic discovery with WeaveDNS](#dns)

### <a name="virtual-ethernet-switch"></a>Virtual Ethernet Switch

To application containers, the network established by weave looks
like a giant Ethernet switch to which all the containers are
connected.

Containers can easily access services from each other; e.g. in the
container on `$HOST1` we can start a netcat "service" with

    root@28841bd02eff:/# nc -lk -p 4422

and then connect to it from the container on `$HOST2` with

    root@f76829496120:/# echo 'Hello, world.' | nc 10.2.1.1 4422

Note that *any* protocol is supported. Doesn't even have to be over
TCP/IP, e.g. a netcat UDP service would be run with

    root@28841bd02eff:/# nc -lu -p 5533

    root@f76829496120:/# echo 'Hello, world.' | nc -u 10.2.1.1 5533

We can deploy the entire arsenal of standard network tools and
applications, developed over decades, to configure, secure, monitor,
and troubleshoot our container network. To put it another way, we can
now re-use the same tools and techniques when deploying applications
as containers as we would have done when deploying them 'on metal' in
our data centre.

### <a name="application-isolation"></a>Application isolation

A single weave network can host multiple, isolated applications, with
each application's containers being able to communicate with each
other but not containers of other applications.

To accomplish that, we assign each application a different subnet. So,
in the above example, if we wanted to add another application similar
to, but isolated from, our first, we'd launch the containers with...

    host1$ D=$(weave run 10.2.2.1/24 -t -i ubuntu)
    host2$ D=$(weave run 10.2.2.2/24 -t -i ubuntu)

A quick 'ping' test in the containers confirms that they can talk to
each other but not the containers of our first application...

    host1$ docker attach $D
    
    root@da50502598d5:/# ping -c 1 -q 10.2.2.2
    PING 10.2.2.2 (10.2.2.2): 48 data bytes
    --- 10.2.2.2 ping statistics ---
    1 packets transmitted, 1 packets received, 0% packet loss
    round-trip min/avg/max/stddev = 0.562/0.562/0.562/0.000 ms
    
    root@da50502598d5:/# ping -c 1 -q 10.2.1.1
    PING 10.2.1.1 (10.2.1.1) 56(84) bytes of data.
    --- 10.2.1.1 ping statistics ---
    1 packets transmitted, 0 received, 100% packet loss, time 0ms
    
    root@da50502598d5:/# ping -c 1 -q 10.2.1.2
    PING 10.2.1.2 (10.2.1.2) 56(84) bytes of data.
    --- 10.2.1.2 ping statistics ---
    1 packets transmitted, 0 received, 100% packet loss, time 0ms

This isolation-through-subnets scheme is an example of carrying over a
well-known technique from the 'on metal' days to containers.

If desired, a container can be attached to multiple subnets when it is
started:

    host1$ weave run 10.2.2.1/24 10.2.3.1/24 -t -i ubuntu

NB: By default docker permits communication between containers on the
same host, via their docker-assigned IP addresses. For complete
isolation between application containers, that feature needs to be
disabled by
[setting `--icc=false`](https://docs.docker.com/articles/networking/#between-containers)
in the docker daemon configuration. Furthermore, containers should be
prevented from capturing and injecting raw network packets - this can
be accomplished by starting them with the `--cap-drop net_raw` option.

### <a name="dynamic-network-attachment"></a>Dynamic network attachment

In some scenarios containers are started independently, e.g. via some
existing tool chain, or require more complex startup sequences than
provided by `weave run`. And sometimes the decision which application
network a container should be part of is made post-startup. For these
situations, weave allows an existing, running container to be attached
to the weave network. To illustrate, we can achieve the same effect as
the first example with

    host1$ C=$(docker run -d -t -i ubuntu)
    host1$ weave attach 10.2.1.1/24 $C

There is a matching `weave detach` command:

    host1$ weave detach 10.2.1.1/24 $C

You can detach a container from one application network and attach it
to another:

    host1$ weave detach 10.2.1.1/24 $C
    host1$ weave attach 10.2.2.1/24 $C

or attach a container to multiple application networks, effectively
sharing it between applications:

    host1$ weave attach 10.2.1.1/24 $C
    host1$ weave attach 10.2.2.1/24 $C

Finally, multiple addresses can be attached or detached with a single
invocation:

    host1$ weave attach 10.2.1.1/24 10.2.2.1/24 10.2.3.1/24 $C
    host1$ weave detach 10.2.1.1/24 10.2.2.1/24 10.2.3.1/24 $C

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

The same password must be specified for all weave peers; it is a
component in the creation of ephemeral session keys for connections
between peers. See the [crypto documentation](how-it-works.html#crypto)
for more details.

### <a name="host-network-integration"></a>Host network integration

Weave application networks can be integrated with a host's network,
establishing connectivity between the host and application containers
anywhere.

Let's say that in our example we want `$HOST2` to have access to the
application containers. On `$HOST2` we run

    host2$ weave expose 10.2.1.102/24

choosing an unused IP address in the application subnet. (There is a
corresponding 'hide' command to revert this step.)

Now

    host2$ ping 10.2.1.2

will work. And, more interestingly,

    host2$ ping 10.2.1.1

will work too, which is talking to a container that resides on `$HOST1`.

Multiple subnet addresses can be exposed or hidden with a single
invocation:

    host2$ weave expose 10.2.1.102/24 10.2.2.102/24
    host2$ weave hide   10.2.1.102/24 10.2.2.102/24

Finally, exposed addresses can be added to weaveDNS by supplying a
fully-qualified domain name:

    host2$ weave expose 10.2.1.102/24 -h exposed.weave.local

### <a name="service-export"></a>Service export

Services running in containers on a weave network can be made
accessible to the outside world (and, more generally, other networks)
from any weave host, irrespective of where the service containers are
located.

Say we want to make our example netcat "service", which is running in
a container on `$HOST1`, accessible to the outside world via `$HOST2`.

First we need to expose the application network to `$HOST2`, as
explained [above](#host-network-integration), i.e.

    host2$ weave expose 10.2.1.102/24

Then we add a NAT rule to route from the outside world to the
destination container service.

    host2$ iptables -t nat -A PREROUTING -p tcp -i eth0 --dport 2211 \
           -j DNAT --to-destination 10.2.1.1:4422

Here we are assuming that the "outside world" is connecting to `$HOST2`
via 'eth0'. We want TCP traffic to port 2211 on the external IPs to be
routed to our 'nc' service, which is running on port 4422 in the
container with IP 10.2.1.1.

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

    host1$ weave expose 10.2.1.101/24

Then we add a NAT rule to route from the above IP to the destination
service.

    host1$ iptables -t nat -A PREROUTING -p tcp -d 10.2.1.101 --dport 3322 \
           -j DNAT --to-destination $HOST3:2211

This allows any application container to reach the service by
connecting to 10.2.1.101:3322. So if `$HOST3` is indeed running a netcat
service on port 2211, e.g.

    host3$ nc -lk -p 2211

then we can connect to it from our application container on `$HOST2` with

    root@f76829496120:/# echo 'Hello, world.' | nc 10.2.1.101 3322

The same command will work from any application container.

### <a name="service-binding"></a>Service binding

Importing a service provides a degree of indirection that allows late
and dynamic binding, similar to what can be achieved with a proxy. In
our example, application containers are unaware that the service they
are accessing at `10.2.1.101:3322` is in fact residing on
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

    host2$ weave expose 10.2.1.102/24

and then adding a NAT rule which routes traffic from the `$HOST2`
network (i.e. anything which can connect to `$HOST2`) to the service
endpoint in the weave network

    host2$ iptables -t nat -A PREROUTING -p tcp -i eth0 --dport 4433 \
           -j DNAT --to-destination 10.2.1.101:3322

Now any host on the same network as `$HOST2` can access the service with

    echo 'Hello, world.' | nc $HOST2 4433

Furthermore, as explained in [service-binding](#service-binding), we
can dynamically alter the service locations without having to touch
the applications that access them, e.g. we could move the example
netcat service to `$HOST4:2211` while retaining its 10.2.1.101:3322
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

### <a name="dns"></a>Automatic discovery with WeaveDNS

WeaveDNS is a distributed DNS service for weave networks, enabling
containers to address each other by name rather than IP address. Find
out more about WeaveDNS from its
[documentation](weavedns-readme.html).
