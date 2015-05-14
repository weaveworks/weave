---
title: Experimental features
layout: default
---

# Experimental features

 * [Proxy](#proxy)

### <a name="proxy"></a>Proxy

Instead of the weave command-line utility, you may prefer to use the standard
docker command-line utility to manage containers. This will allow containers to
be started in foreground mode. It will also ensure that the weave network
interface is available before running your container.

#### Setup

To do this, while still connecting containers to the weave network, you can use
the weave proxy. We can start the proxy with

    host1$ weave launch-proxy

By default, the proxy will connect to docker at `unix:///var/run/docker.sock`,
and listen on port 12375. However, we can adjust the connection to docker via
the `-H` argument. All docker commands can be run via the proxy, so it is safe
to globally adjust your `DOCKER_HOST`.

    host1$ export DOCKER_HOST="tcp://host1:12375"
    host1$ docker ps

#### Usage

When containers are created via the weave proxy, their entrypoint will be
modified to wait for the weave network interface to become available. When they are
started via the weave proxy, any IP addresses and networks specified in the
`WEAVE_CIDR` environment variable will be attached. We can create and start a
container via the weave proxy with

    host1$ docker run -e WEAVE_CIDR=10.2.1.1/24 -ti ubuntu /bin/sh

or, equivalently with

    host1$ docker create -e WEAVE_CIDR=10.2.1.1/24 -ti ubuntu /bin/sh
    5ef831df61d50a1a49272357155a976595e7268e590f0a2c75693337b14e1382
    host1$ docker start 5ef831df61d50a1a49272357155a976595e7268e590f0a2c75693337b14e1382

Multiple IP addresses and networks can be supplied in the WEAVE_CIDR variable by space-separating them, as in `WEAVE_CIDR=10.2.1.1/24 10.2.2.1/24`.

#### Usage with WeaveDNS

Containers started via the proxy can be automatically configured to use WeaveDNS for name resolution. To accomplish this we need to launch the proxy with the `--with-dns` option

    host1$ weave launch
    host1$ weave launch-dns 10.2.254.1/24
    host1$ weave launch-proxy --with-dns

With this done, any containers launched through the proxy will use weaveDNS for name resolution. WeaveDNS is used in addition to any dns servers specified via the `--dns` option. More details on weaveDNS can be found in the [weaveDNS documentation](weavedns-readme.html).

More details on weaveDNS can be found in the [weaveDNS documentation](weavedns-readme.html).

#### Limitations

* The proxy does not currently support TLS.
* If you have a firewall, you will need to make sure port 12375 is open.
