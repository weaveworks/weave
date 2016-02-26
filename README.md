# Weave - weaving containers into applications

[![Build Status](https://travis-ci.org/weaveworks/weave.svg?branch=master)](https://travis-ci.org/weaveworks/weave) [![Integration Tests](https://circleci.com/gh/weaveworks/weave/tree/master.svg?style=shield&circle-token=4933c7dabb3d0383e62117565cb9d16df7b1a811)](https://circleci.com/gh/weaveworks/weave) [![Coverage Status](https://coveralls.io/repos/weaveworks/weave/badge.svg)](https://coveralls.io/r/weaveworks/weave)

# About Weaveworks

[Weaveworks](http://weave.works) is the company that develops Weave -
the most productive way for developers to connect, observe and control
Docker containers. To learn about our products, including getting
started tutorials, visit our [website](http://weave.works) and
[documentation](http://docs.weave.works) or continue to read about
some of the more technical aspects of Weave in this readme.

# Weave

Weave creates a virtual network that connects Docker containers
deployed across multiple hosts and enables their automatic discovery.

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

Ensure you are running Linux (kernel 3.8 or later) and have Docker
(version 1.6.0 or later) installed. Then install weave with

    sudo curl -L git.io/weave -o /usr/local/bin/weave
    sudo chmod a+x /usr/local/bin/weave

For usage on OSX (with Docker Machine) you first need to
[make sure that a VM is running and configured](https://docs.docker.com/installation/mac/#from-your-shell).
Then you can launch weave directly from the OSX host.

For installing weave on other platforms, follow the [integration guides](http://weave.works/product/integrations/).

Weave respects the environment variable `DOCKER_HOST`, so you can run
it locally to control a weave network on a remote host.

Weave will periodically check with our servers to see if a new version is available.
New versions are announced in the log. To disable this, run:

```
CHECKPOINT_DISABLE=true weave launch
```

For more information, [read this](https://github.com/weaveworks/go-checkpoint).

## Quick Start Screencast

<a href="https://youtu.be/kihQCCT1ykE" alt="Click to watch the screencast" target="_blank">
  <img src="/docs/hello-screencast.png" />
</a>

## Example

Say you have docker running on two hosts, accessible to each other as
`$HOST1` and `$HOST2`, and want to deploy an application consisting of
two containers, one on each host.

On $HOST1 we run:

    host1$ weave launch
    host1$ eval $(weave env)
    host1$ docker run --name a1 -ti ubuntu

> NB: If the first command results in an error like
> `http:///var/run/docker.sock/v1.19/containers/create: dial unix
> /var/run/docker.sock: permission denied. Are you trying to connect
> to a TLS-enabled daemon without TLS?` then you likely need to be
> 'root' in order to connect to the Docker daemon. If so, run the
> above and all subsequent commands in a *single* root shell (e.g. one
> created with `sudo -s`). Do *not* prefix individual commands with
> `sudo`, since some commands modify environment entries and hence
> they all need to be executed from the same shell.

The first line runs weave. The second line configures our environment
so that containers launched via the docker command line are
automatically attached to the weave network. Finally, we run our
application container.

That's it! If our application consists of more than one container on
this host we simply launch them with `docker run` as appropriate.

Next we repeat similar steps on `$HOST2`...

    host2$ weave launch $HOST1
    host2$ eval $(weave env)
    host2$ docker run --name a2 -ti ubuntu

The only difference, apart from the name of the application container,
is that we tell our weave that it should peer with the weave on
`$HOST1` (specified as the IP address or hostname, and optional
`:port`, by which `$HOST2` can reach it). NB: if there is a firewall
between `$HOST1` and `$HOST2`, you must permit traffic to the weave
control port (TCP 6783) and data ports (UDP 6783/6784).

Note that we could instead have told the weave on `$HOST1` to connect to
`$HOST2`, or told both about each other. Order does not matter here;
weave automatically (re)connects to peers when they become
available. Also, we can tell weave to connect to multiple peers by
supplying multiple addresses, separated by spaces. And we can
[add peers dynamically](http://docs.weave.works/weave/latest_release/features.html#dynamic-topologies).

Weave must be started once per host. The relevant container images are
pulled down on demand, but if you wish you can preload them by running
`weave setup` - this is particularly useful for automated deployments,
and ensures that there are no delays during later operations.

Now that we've got everything set up, let's see whether our containers
can talk to each other...

In the container started on `$HOST1`...

    root@a1:/# ping -c 1 -q a2
    PING a2.weave.local (10.40.0.2) 56(84) bytes of data.
    --- a2.weave.local ping statistics ---
    1 packets transmitted, 1 received, 0% packet loss, time 0ms
    rtt min/avg/max/mdev = 0.341/0.341/0.341/0.000 ms

Similarly, in the container started on `$HOST2`...

    root@a2:/# ping -c 1 -q a1
    PING a1.weave.local (10.32.0.2) 56(84) bytes of data.
    --- a1.weave.local ping statistics ---
    1 packets transmitted, 1 received, 0% packet loss, time 0ms
    rtt min/avg/max/mdev = 0.366/0.366/0.366/0.000 ms

So there we have it, two containers on separate hosts happily talking
to each other.

## Find out more

 * [Documentation homepage](http://docs.weave.works/weave/latest_release/)
 * [Getting Started Guides](http://weave.works/guides/)
 * [Features](http://docs.weave.works/weave/latest_release/features.html)
 * [Troubleshooting](http://docs.weave.works/weave/latest_release/troubleshooting.html)
 * [Building](http://docs.weave.works/weave/latest_release/building.html)
 * [How it works](http://docs.weave.works/weave/latest_release/how-it-works.html)

## Contact Us

Found a bug, want to suggest a feature, or have a question?  Please
[file an issue](https://github.com/weaveworks/weave/issues), or post
to the
[Weave Users Google Group](https://groups.google.com/a/weave.works/forum/#!forum/weave-users),
which you can email at weave-users@weave.works.  Or you can contact
Weaveworks directly at help@weave.works. When reporting a bug, please
include which version of weave you are running, as shown by `weave
version`.

Follow on Twitter:
[@weaveworks](https://twitter.com/weaveworks).

Read the Weave blog:
[Weaveblog](http://weaveblog.com/).

IRC:
[#weavenetwork](https://botbot.me/freenode/weavenetwork/)
