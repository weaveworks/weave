# Weave - the Docker network

Weave creates a virtual network that connects Docker containers
deployed across multiple hosts.

![Weave Virtual Network](/docs/virtual-network.png?raw=true "Weave Virtual Network")

Applications use the network just as if the containers were all
plugged into the same network switch, with no need to configure port
mappings, links, etc. Services provided by application containers on
the weave network can be made accessible to the outside world,
regardless of where those containers are running. Similarly, existing
internal systems can be exposed to application containers irrespective
of their location.

![Weave Deployment](/docs/deployment.png?raw=true "Weave Deployment")

Weave can traverse firewalls and operate in partially connected
networks. Traffic can be encrypted, allowing hosts to be connected
across an untrusted network.

With weave you can easily construct applications consisting of
multiple containers, running anywhere.

Weave works alongside Docker's existing (single host) networking
capabilities, so these can continue to be used by containers.

## Installation

To run weave on a host, you need to install...

1. docker. We've tested with versions 0.9.1 through 1.2.0, but other
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
$HOST1 and $HOST2, and want to deploy an application consisting of two
containers, one on each host.

On $HOST1 run (as root)

    host1# weave launch 10.0.0.1/16
    host1# C=$(weave run 10.0.1.1/24 -t -i ubuntu)

The first line starts the weave router, in a container. This needs to
be done once on each host. We tell weave that its IP address should
be 10.0.0.1, and that the weave network is on 10.0.0.0/16.

The second line starts our application container. We give it an IP
address and network (a subnet of the weave network). `weave run`
invokes `docker run -d` with all the parameters following the IP
address and netmask. So we could be launching any container this way;
here we just take a stock ubuntu container and launch a shell in it.

If our application consists of more than one container on this host we
simply launch them with a variation on that second line.

The IP addresses and netmasks can be anything you like which doesn't
conflict with any IP ranges of 'external' services the hosts or your
containers need to connect to. The same IP range must be used
everywhere, and the individual IP addresses must, of course, be
unique.

We repeat similar steps on $HOST2...

    host2# weave launch 10.0.0.2/16 $HOST1
    host2# C=$(weave run 10.0.1.2/24 -t -i ubuntu)

The only difference, apart from the choice of IP addresses for the
weave router and the application container, is that we tell our weave
that it should peer with the weave on $HOST1 (specified as the IP
address or hostname by which $HOST2 can reach it). NB: if there is a
firewall between $HOST1 and $HOST2, you must open port 6783 for TCP
and UDP.

Note that we could instead have told the weave on $HOST1 to connect to
$HOST2, or told both about each other. Order does not matter here;
weave automatically (re)connects to peers when they become
available. Also, we can tell weave to connect to multiple peers by
supplying multiple addresses, separated by spaces. And we can
[add peers dynamically](#dynamic-topologies).

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

## Find out more

 * [Documentation homepage](http://zettio.github.io/weave/)
 * [Features](http://zettio.github.io/weave/features.html)
 * [Troubleshooting](http://zettio.github.io/weave/troubleshooting.html)
 * [Building](http://zettio.github.io/weave/building.html)
 * [How it works](http://zettio.github.io/weave/how-it-works.html)

## Contact Us

Found a bug, want to suggest a feature, or have a question?
[File an issue](https://github.com/zettio/weave/issues), or email
weave@zett.io. When reporting a bug, please include which version of
weave you are running, as shown by `weave version`.

Follow weave on Twitter:
[@weavenetwork](https://twitter.com/weavenetwork).

Read the Weave blog:
[Weaveblog](http://weaveblog.com/).
