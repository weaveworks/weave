# Weave - the Docker network

Weave creates a virtual network that connects Docker containers
deployed across multiple hosts.

Applications use the network just as if the containers were all
plugged into the same network switch, with no need to configure port
mappings, links, etc. Weave can optionally encrypt traffic, allowing
hosts to be connected across an untrusted network.

Services provided by application containers on the weave network can
be made accessible to the outside world, regardless of where those
containers are running.

With weave you can easily construct applications consisting of
multiple containers, running anywhere.

Weave works alongside Docker's existing (single host) networking
capabilities, so these can continue to be used by containers.

## Installation

To run weave on a host, you need to install...

1. docker. We've tested with version 1.1.0 through 1.2.0, but other
   versions should work too.
2. weave. Install this with

        sudo wget -O /usr/local/bin/weave \
          https://raw.githubusercontent.com/zettio/weave/master/weaver/weave
        sudo chmod a+x /usr/local/bin/weave

3. (recommended) ethtool. On many systems this is installed already;
   if not then grab it via your favourite package manager. On some
   systems, weave application container networking may not operate
   correctly unless ethtool is available.

4. (optional) conntrack. Install this via your favourite package
   manager. Without conntrack, the weave network may not re-establish
   itself fully when individual weave instances are stopped (with
   `weave stop`) and restarted quickly (typically within ~3 minutes).

## Example

Say you have docker running on two hosts, accessible to each other as
$HOST1 and $HOST2, and want to deploy an application consisting of
two containers, one on each host.

On $HOST1 run (as root)

    host1# weave launch 10.0.0.1/16
    host1# C=$(weave run 10.0.1.1/24 -t -i ubuntu /bin/bash)

The first line starts the weave router, in a container. This needs to
be done once on each host. We tell weave that its IP address should
be 10.0.0.1, and that the weave network is on 10.0.0.0/16.

The second line starts our application container. We give it an IP
address and network (a subnet of the weave network). `weave run`
invokes `docker run -d` with all the parameter following the IP
address and netmask. So we could be launching any container this way;
here we just take a stock ubuntu container and launch a shell in it.

If our application consists of more than one container on this host we
simply launch them with a variation on that 2nd line.

The IP addresses and netmasks can be anything you like which doesn't
conflict with any IP ranges of 'external' services the hosts or your
containers need to connect to. The same IP range must be used
everywhere, and the individual IP addresses must, of course, be
unique.

We repeat similar steps on $HOST2...

    host2# weave launch 10.0.0.2/16 $HOST1
    host2# C=$(weave run 10.0.1.2/24 -t -i ubuntu /bin/bash)

The only difference, apart from the IP addresses, is that we tell our
weave that it should peer with the weave running on $HOST1. We could
instead have told the weave on $HOST1 to connect to $HOST2, or told
both about each other. Order doesn't matter here; weave automatically
(re)connects to peers when they become available. Also, we can tell
weave to connect to multiple peers by supplying multiple
addresses. And we can [add peers dynamically](#dynamic-topologies).

Now that we've got everything set up, let's see whether our containers
can talk to each other...

On $HOST1...

    host1# docker attach $C
    root@28841bd02eff:/# ping -c 1 -q 10.0.1.2
    PING 10.0.1.2 (10.0.1.2): 48 data bytes
    --- 10.0.1.2 ping statistics ---
    1 packets transmitted, 1 packets received, 0% packet loss
    round-trip min/avg/max/stddev = 1.048/1.048/1.048/0.000 ms

Similarly, on $HOST2...

    host2# docker attach $C
    root@f76829496120:/# ping -c 1 -q 10.0.1.1
    PING 10.0.1.1 (10.0.1.1): 48 data bytes
    --- 10.0.1.1 ping statistics ---
    1 packets transmitted, 1 packets received, 0% packet loss
    round-trip min/avg/max/stddev = 1.034/1.034/1.034/0.000 ms

So there we have it, two containers on separate hosts happily talking
to each other.

## Features

Weave has a few more features beyond those illustrated by the basic
example above...

### Virtual Ethernet Switch

To application containers, the network established by weave looks
like a giant Ethernet switch to which all the containers are
connected.

Containers can easily access services from each other, e.g. in the
container on $HOST1 we can start a netcat "service" with

    root@28841bd02eff:/# nc -lk -p 4422

and then connect to it from the container on $HOST2 with

    root@f76829496120:/# echo 'Hello, world.' | nc 10.0.1.1 4422

Note that *any* protocol is supported. Doesn't even have to be over
TCP/IP, e.g. a netcat UDP service would be run with

    root@28841bd02eff:/# nc -lu -p 5533

    root@f76829496120:/# echo 'Hello, world.' | nc -u 10.0.1.1 5533

We can deploy the entire arsenal of standard network tools and
applications, developed over decades, to configure, secure, monitor,
and troubleshoot our container network. To put it another way, we can
now re-use the same tools and techniques when deploying applications
as containers as we would have done when deploying them 'on metal' in
our data centre.

### Application isolation

A single weave network can host multiple, isolated applications, with
each application's containers being able to communicate with each
other but not containers of other applications.

To accomplish that, we assign each application a different subnet. So,
in the above example, if we wanted to add another application similar
to, but isolated from, our first, we'd launch the containers with...

    host1# D=$(weave run 10.0.2.1/24 -t -i ubuntu /bin/bash)
    host2# D=$(weave run 10.0.2.2/24 -t -i ubuntu /bin/bash)

A quick 'ping' test in the containers confirms that they can talk to
each other but not the containers of our first application...

    host1# docker attach $D
    root@da50502598d5:/# ping -c 1 -q 10.0.2.2
    PING 10.0.2.2 (10.0.2.2): 48 data bytes
    --- 10.0.2.2 ping statistics ---
    1 packets transmitted, 1 packets received, 0% packet loss
    round-trip min/avg/max/stddev = 0.562/0.562/0.562/0.000 ms
    root@da50502598d5:/# ping -c 1 -q 10.0.1.1
    PING 10.0.1.1 (10.0.1.1) 56(84) bytes of data.
    --- 10.0.1.1 ping statistics ---
    1 packets transmitted, 0 received, 100% packet loss, time 0ms
    root@da50502598d5:/# ping -c 1 -q 10.0.1.2
    PING 10.0.1.2 (10.0.1.2) 56(84) bytes of data.
    --- 10.0.1.2 ping statistics ---
    1 packets transmitted, 0 received, 100% packet loss, time 0ms

This isolation-through-subnets scheme is an example of carrying over a
well-known technique from the 'on metal' days to containers.

### Security

In order to connect containers across untrusted networks, weave peers
can be told to encrypt traffic by supplying a `-password` option when
launching weave, e.g.

    host1# weave launch 10.0.0.1/16 -password wEaVe

The same password must be specified for all weave peers; it is a
component in the creation of ephemeral session keys for connections
between peers.

### Host network integration

Weave application networks can be integrated with a host's network,
establishing connectivity between the host and application containers
anywhere.

Let's say that in our example we we want $HOST2 to have access to the
application containers. On $HOST2 we run

    host2# weave expose 10.0.1.102/24

choosing an unused IP address in the application subnet. (There is a
corresponding 'hide' command to revert this step.)

Now

    host2# ping 10.0.1.2

will work. And, more interestingly,

    host2# ping 10.0.1.1

will work too, which is talking to a container that resides on $HOST1.

### Service export

Services running in containers on a weave network can be made
accessible to the outside world (and, more generally, other networks)
from any weave host, irrespective of where the service containers are
located.

Say we want to make our example netcat "service", which is running in
a container on $HOST1, accessible to the outside world via $HOST2.

First we need to expose the application network to $HOST2, as
explained [above](#host-network-integration), i.e.

    host2# weave expose 10.0.1.102/24

Then we add a NAT rule to route from the outside world to the
destination container service.

    host2# iptables -t nat -A PREROUTING -p tcp -i eth0 --dport 2211 \
           -j DNAT --to-destination 10.0.1.1:4422

Here we are assuming that the "outside world" is connecting to $HOST2
via 'eth0'. We want TCP traffic to port 2211 on the external IPs to be
routed to our 'nc' service, which is running on port 4422 in the
container with IP 10.0.1.1.

With the above in place, we can connect to our 'nc' service from
anywhere with

    echo 'Hello, world.' | nc $HOST2 2211

(NB: due to the way routing is handled in the Linux kernel, this won't
work when run *on* $HOST2.)

Similar NAT rules to the above can used to expose services not just to
outside world but also other, internal, networks.

### Service import

Applications running in containers on a weave network can be given
access to services which are only reachable from certain weave hosts,
irrespective of where the application containers are located.

Say that, as an extension of our example, we have a netcat service
running on $HOST3, port 2211, and that $HOST3 is not part of the weave
network and is only reachable from $HOST1, not $HOST2. Nevertheless we
want to make the service accessible to an application running in a
container on $HOST2.

First we need to expose the application network to the host, as
explained [above](#host-network-integration), this time on $HOST1,
i.e.

    host1# weave expose 10.0.1.101/24

Then we add a NAT rule to route from the above IP to the destination
service.

    host1# iptables -t nat -A PREROUTING -p tcp -d 10.0.1.101 --dport 3322 \
           -j DNAT --to-destination $HOST3:2211

This allows any application container to reach the service by
connecting to 10.0.0.1:3322. So if $HOST3 is indeed running a netcat
service on port 2211, e.g.

    host3# nc -kl 2211

then we can connect to it from our application container on $HOST2 with

    root@f76829496120:/# echo 'Hello, world.' | nc 10.0.1.101 3322

The same command will work from any application container.

### Multi-cloud networking

Weave can network containers hosted in different cloud providers /
data centres. So, for example, one could run an application consisting
of containers on GCE, AMZN and a local data centres.

To enable this, the network must be configured to permit TCP and UDP
connections to port 6783 of the docker hosts.

### Multi-hop routing

A network of containers across more than two hosts can be established
even when there is only partial connectivity between the hosts. Weave
is able to route traffic between containers as long as there is at
least one *path* of connected hosts between them.

So, for example, if a docker host in a local data centre can connect
to hosts in GCE and AMZN, but the latter two cannot connect to each
other, containers in the latter two can still communicate; weave will
route the traffic via the local data centre.

### Dynamic topologies

To add a host to an existing weave network, one simply launches
weave on the host, supplying the address of at least one existing
host. Weave will automatically discover the other hosts in the other
network and establish connections to them if it can (in order to avoid
unnecessary multi-hop routing).

### Container mobility

Containers can be moved between hosts without requiring any
reconfiguration or, in many cases, restarts of other containers. All
that is required is for the migrated container to be started with the
same IP address as it was given originally.

### Fault tolerance

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

## Troubleshooting

Check the weave container logs with

    docker logs weave

A reasonable amount of information, and all errors, get logged there.

The log verbosity can be increased by supplying the `-debug` flag when
launching weave. Be warned, this will log information on a per-packet
basis, so can produce a lot of output.

One can ask a weave router to report its status with

    weave status

## Installation with Boot2Docker

If you are running Docker inside the Boot2Docker VM, e.g. because you
are on a Mac, then the following changes are required to these
instructions:

Assuming you have fetched the 'weave' script via curl or similar, and
it is in the current directory, transfer it to the Boot2Docker VM and
make it executable like this:

    host1$ boot2docker ssh "cat > weave" < weave
    host1$ boot2docker ssh "chmod a+x weave"

Then, if we were trying to create the same containers as in the first
example above, the 'launch' command would be run like this:

    host1$ boot2docker ssh "sudo ./weave launch 10.0.0.1/16"

and the 'run' command like this:

    host1# C=$(boot2docker ssh "sudo ./weave run 10.0.1.1/24 -t -i ubuntu /bin/bash")

## Building

(NB. This is only necessary if you want to work on weave. Also, these
instructions have only been tested on Ubuntu.)

To build weave you need `libpcap-dev` and `docker` installed. And
`go`.

Then simply run

    $ make -C weaver

This will build the weave router, produce a docker image
`zettio/weave` and export that image to /tmp/weave.tar

## How does it work?

Weave creates a network bridge on the host. Each container is
connected to that bridge via a veth pair, the container side of which
is given the IP address & netmask supplied in 'weave run'. Also
connected to the bridge is the weave router container, which is given
the IP address & netmask supplied in 'weave launch'.

A weave router captures Ethernet packets from its bridge-connected
interface in promiscuous mode, using 'pcap'. It forwards these packets
over UDP to weave router peers running on other hosts. On receipt of
such a packet, a router injects the packet on its bridge interface
using 'pcap' and/or forwards the packet to peers.

Weave routers learn which peer host a particular MAC address resides
on. They combine this knowledge with topology information in order to
make routing decisions and thus avoid forwarding every packet to every
peer. The topology information captures which peers are connected to
which other peers; weave can route packets in partially connected
networks with changing topology.

Weave routers establish TCP connections to each other, over which they
perform a protocol handshake and subsequently exchange topology
information. These connections are encrypted if so configured. Peers
also establish UDP "connections", possibly encrypted, for the
aforementioned packet forwarding. These "connections" are duplex and
can traverse firewalls.

More details on the inner workings of weave can be found in the
[architecture documentation](docs/architecture.txt).

## Contact Us

Found a bug, want to suggest a feature, or have a question?
[File an issue](https://github.com/zettio/weave/issues), or email us
at weave@zett.io.
