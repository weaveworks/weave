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

Ensure you are running Linux (kernel 3.5 or later) and have Docker
(version 0.9.1 or later) installed. Then install weave with

    sudo wget -O /usr/local/bin/weave \
      https://github.com/zettio/weave/releases/download/latest_release/weave
    sudo chmod a+x /usr/local/bin/weave

CoreOS users see [here](https://github.com/fintanr/weave-gs/blob/master/coreos-simple/user-data) for an example of installing weave using cloud-config.

Weave respects the environment variable `DOCKER_HOST`, so you can run
it locally to control a weave network on a remote host.

## Quick Start Screencast

<a href="http://youtu.be/k6r7yuSr0hE" alt="Click to watch the screencast" target="_blank">
  <img src="/docs/hello-screencast.png" />
</a>

## Example

Say you have docker running on two hosts, accessible to each other as
`$HOST1` and `$HOST2`, and want to deploy an application consisting of
two containers, one on each host.

On `$HOST1` run (as root)

    host1# weave launch
    host1# C=$(weave run 10.2.1.1/24 -t -i ubuntu)

The first line starts the weave router, in a container. This needs to
be done once on each host. The required docker image for the weave
router container is downloaded automatically. There is also a `weave
setup` command for downloading this and other images required for
weave operation; this is a strictly optional step which is especially
useful for automated installation of weave and ensures that any
subsequent weave commands do not encounter delays due to image
downloading.

The second line runs our application container. We give it an IP
address and network, in
[CIDR notation](http://en.wikipedia.org/wiki/Classless_Inter-Domain_Routing#CIDR_notation).
`weave run` invokes `docker run -d` with all the parameters following
the IP address and network. So we could be launching any container
this way; here we just take a stock ubuntu container and launch a
shell in it. There's also a `weave start` command, which invokes
`docker start` for starting existing containers.

If our application consists of more than one container on this host we
simply launch them with a variation on that second line.

The IP addresses and netmasks can be anything you like, but make sure
they don't conflict with any IP ranges in use on the hosts or IP
addresses of external services the hosts or containers need to connect
to. The same IP range must be used everywhere, and the individual IP
addresses must, of course, be unique.

We repeat similar steps on `$HOST2`...

    host2# weave launch $HOST1
    host2# C=$(weave run 10.2.1.2/24 -t -i ubuntu)

The only difference, apart from the choice of IP address for the
application container, is that we tell our weave that it should peer
with the weave on `$HOST1` (specified as the IP address or hostname, and
optional `:port`, by which `$HOST2` can reach it). NB: if there is a
firewall between `$HOST1` and `$HOST2`, you must open port 6783 for TCP
and UDP.

Note that we could instead have told the weave on `$HOST1` to connect to
`$HOST2`, or told both about each other. Order does not matter here;
weave automatically (re)connects to peers when they become
available. Also, we can tell weave to connect to multiple peers by
supplying multiple addresses, separated by spaces. And we can
[add peers dynamically](http://zettio.github.io/weave/features.html#dynamic-topologies).

Now that we've got everything set up, let's see whether our containers
can talk to each other...

On `$HOST1`...

    host1# docker attach $C
    root@28841bd02eff:/# ping -c 1 -q 10.2.1.2
    PING 10.2.1.2 (10.2.1.2): 48 data bytes
    --- 10.2.1.2 ping statistics ---
    1 packets transmitted, 1 packets received, 0% packet loss
    round-trip min/avg/max/stddev = 1.048/1.048/1.048/0.000 ms

Similarly, on `$HOST2`...

    host2# docker attach $C
    root@f76829496120:/# ping -c 1 -q 10.2.1.1
    PING 10.2.1.1 (10.2.1.1): 48 data bytes
    --- 10.2.1.1 ping statistics ---
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
 * [WeaveDNS README](https://github.com/zettio/weave/tree/master/weavedns#readme)

## Contact Us

Found a bug, want to suggest a feature, or have a question?
[File an issue](https://github.com/zettio/weave/issues), or email
help@weave.works. When reporting a bug, please include which version of
weave you are running, as shown by `weave version`.

Follow weave on Twitter:
[@weaveworks](https://twitter.com/weaveworks).

Read the Weave blog:
[Weaveblog](http://weaveblog.com/).

IRC:
[#weavenetwork](https://botbot.me/freenode/weavenetwork/)
