# Weave - the Docker SDN

Weave lets you connect Docker containers deployed across multiple
hosts, as if they were all plugged into the same network switch.

## Installation

Each host needs to have the following installed

- `wedo` and `docker-ns` from the weave sub-directory. Install these
  somewhere in the root user's path, e.g. `/usr/local/bin'.
- [pipework][], by Jérôme Petazzoni. Install this in the same place as
  the aforementioned scripts.

[pipework]: https://raw.githubusercontent.com/jpetazzo/pipework/master/pipework

e.g.

    host# cp -a weave/wedo weave/docker-ns /usr/local/bin
    host# curl https://raw.githubusercontent.com/jpetazzo/pipework/master/pipework > /usr/local/bin/pipework; chmod a+x /usr/local/bin/pipework

We also need `ethtool` and `arping`. On most systems these are
installed already; if not then grab them via your favourite package
manager.

## Example

Say you have docker running on two hosts, accessible to each other as
$HOST1 and $HOST2, and want to deploy an application consisting of
two containers, one on each host.

On $HOST1 run (as root)

    host1# WEAVE=$(wedo launch 10.0.0.1/16)
    host1# C=$(wedo run 10.0.1.1/24 -t -i debian /bin/bash)

The first line starts the weave router, in a container. This needs to
be done once on each host. We tell weave that its IP address should
be 10.0.0.1, and that the weave network is on 10.0.0.0/16.

The second line starts our application container. We give it an IP
address and network (a subnet of the weave network). `wedo run`
invokes `docker run -d` with all the parameter following the IP
address and netmask. So we could be launching any container this way;
here we just take a stock debian container and launch a shell in it.

If our application consists of more than one container on this host we
simply launch them with a variation on that 2nd line.

The IP addresses and netmasks can be anything you like which doesn't
conflict with any IP ranges of 'external' services your containers
need to connect to. The same IP range must be used everywhere, and the
individual IP addresses must, of course, be unique.

We repeat similar steps on $HOST2...

    host2# WEAVE=$(wedo launch 10.0.0.2/16 $HOST1)
    host2# C=$(wedo run 10.0.1.2/24 -t -i debian /bin/bash)

The only difference, apart from the IP addresses, is that we tell our
weave that it should peer with the weave running on $HOST1. We could
instead have told the weave on $HOST1 to connect to $HOST2, or told
both about each other. Order doesn't matter here; weave automatically
(re)connects to peers when they are become available. Also, we can
tell weave to connect to multiple peers by supplying multiple
addresses.

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

That means *any* protocol is supported. Doesn't even have to be over
TCP/IP, e.g. in the above example,

    root@28841bd02eff:/# nc -lu -p 4422

    root@f76829496120:/# echo 'Hello, world.' | nc -u 10.0.1.1 4422

sends some data from the 2nd container to the first over a UDP
connection on port 4422.

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

    host1# D=$(wedo run 10.0.2.1/24 -t -i debian /bin/bash)
    host2# D=$(wedo run 10.0.2.2/24 -t -i debian /bin/bash)

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

    host1# WEAVE=$(wedo launch 10.0.0.1/16 -password wEaVe)

The same password must be specified for all weave peers.

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

### Fault tolerance

Weave peers continually exchange topology information, and monitor
and (re)establish network connections to other peers. So if hosts or
networks fail, weave can "route around" the problem. This includes
network partitions; containers on either side of a partition can
continue to communicate, with full connectivity being restored when
the partition heals.

The weave container is very light-weight - just over 8MB image size
and a few 10s of MBs of runtime memory - and disposable. I.e. should
weave ever run into difficulty, one can simply bounce the weave
container. Application containers do *not* have to be restarted in
that event, and indeed may not even experience a temporary
connectivity failure if the weave container is restarted quickly
enough.

## Building

(NB. This is only necessary if you want to work on weave.)

To build weave you need `libpcap-dev` and `docker` installed.

Then simply run

    $ make -C weave

This will build the weave application, produce a docker image
`zettio/weave` and export that image to /tmp/wedo.tar
