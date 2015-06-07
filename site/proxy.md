---
title: Weave Proxy
layout: default
---

# Weave Proxy

The proxy automatically attaches containers to the weave network when
they are started using the ordinary Docker
[command-line interface](https://docs.docker.com/reference/commandline/cli/)
or
[remote API](https://docs.docker.com/reference/api/docker_remote_api/),
instead of `weave run`.

 * [Setup](#setup)
 * [Usage](#usage)
 * [Automatic IP address assignment](#ipam)
 * [Automatic discovery](#dns)
 * [Multi-host example](#multi-host)
 * [Securing the docker communication with TLS](#tls)
 * [Troubleshooting](#troubleshooting)

## <a name="setup"></a>Setup

The proxy sits between the Docker client (command line or API) and the
Docker daemon, intercepting the communication between the two.

To start the proxy, run

    host1$ weave launch-proxy

By default, the proxy listens on port 12375, on all network
interfaces. This can be adjusted with the `-H` argument, e.g.

    host1$ weave launch-proxy -H tcp://localhost:12375

All docker commands can be run via the proxy, so it is safe to
globally adjust your `DOCKER_HOST`, e.g.

    host1$ export DOCKER_HOST=tcp://localhost:12375
    host1$ docker ps
    ...

If you are working with a remote docker daemon, then `localhost` in
the above needs to be replaced with the docker daemon host, and any
firewalls inbetween need to be configured to permit access to
port 12375.

## <a name="usage"></a>Usage

When containers are created via the weave proxy, their entrypoint will
be modified to wait for the weave network interface to become
available. When they are started via the weave proxy, any IP addresses
and networks specified in the `WEAVE_CIDR` environment variable will
be attached. We can create and start a container via the weave proxy
with

    host1$ docker run -e WEAVE_CIDR=10.2.1.1/24 -ti ubuntu

or, equivalently with

    host1$ docker create -e WEAVE_CIDR=10.2.1.1/24 -ti ubuntu
    5ef831df61d50a1a49272357155a976595e7268e590f0a2c75693337b14e1382
    host1$ docker start 5ef831df61d50a1a49272357155a976595e7268e590f0a2c75693337b14e1382

Multiple IP addresses and networks can be supplied in the `WEAVE_CIDR`
variable by space-separating them, as in
`WEAVE_CIDR="10.2.1.1/24 10.2.2.1/24"`.

Using the proxy brings some additional benefits over `weave run`. The
proxy ensures that the weave network interface is available before
starting a container's application process. Furthermore, containers
can be started in foreground mode, and can be automatically removed
(with the ususal `--rm`).

## <a name="ipam"></a>Automatic IP address assignment

If [automatic IP address assignment](ipam.html) is enabled in weave,
e.g. by launching it with

    host1$ weave launch -iprange 10.2.3.0/24

then containers started via the proxy can be automatically assigned an
IP address by providing a blank `WEAVE_CIDR` environment variable, as
in

    host1$ docker run -e WEAVE_CIDR= -ti ubuntu

Furthermore, it is possible to configure the proxy such that all
containers started via it are automatically assigned an IP address and
attached to weave network, *without having to specify any special
environment variables or other options*. To do this we launch the
proxy with the `--with-ipam` option, e.g.

    host1$ weave launch-proxy --with-ipam

Now any container started via the proxy, e.g. with

    host1$ docker run -ti ubuntu

gets attached to the weave network with an automatically assigned IP
address. Containers started with a `WEAVE_CIDR` environment variable
are handled as before.

## <a name="dns"></a>Automatic discovery

Containers started via the proxy are automatically registered in
[weaveDNS](weavedns.html) if they have a hostname in the weaveDNS
domain (usually `.weave.local`). In order for containers to be able to
look up such names, their DNS resolver needs to be configured to point
at weaveDNS. This can be done [manually](weavedns.html#without-run),
or automatically by the proxy by launching it with the `--with-dns`
option, i.e.

    host1$ weave launch-proxy --with-dns

Now any containers started via the proxy, and with a `WEAVE_CIDR=...`
environment variable (or even without it, if IPAM is
[configured](#ipam)), will use weaveDNS for name resolution.

## <a name="multi-host"></a>Multi-host example

Here's a complete example of using weave proxies configured with
[automatic IP address assignment](#ipam) and
[automatic discovery](#dns) to start containers on two hosts such that
they can reach each other by name.

First, let us start weave, weaveDNS and the proxy, and set DOCKER_HOST
to point at the latter:

    host1$ weave launch -iprange 10.2.3.0/24
    host1$ weave launch-dns 10.2.4.1/24
    host1$ weave launch-proxy --with-ipam --with-dns
    host1$ export DOCKER_HOST=tcp://localhost:12375

    host2$ weave launch -iprange 10.2.3.0/24 host1
    host2$ weave launch-dns 10.2.4.2/24
    host2$ weave launch-proxy --with-ipam --with-dns
    host2$ export DOCKER_HOST=tcp://localhost:12375

NB: Note that the two weaveDNS instances must be given unique IPs, on
a subnet different from that used for IP allocation.

Now let us start a couple of containers, and ping one from the other,
by name.

    host1$ docker run -h pingme.weave.local -dti ubuntu

    host2$ docker run -h pinger.weave.local  -ti ubuntu
    root@pinger:/# ping pingme
    PING pingme.weave.local (10.2.3.1) 56(84) bytes of data.
    64 bytes from pingme.weave.local (10.2.3.1): icmp_seq=1 ttl=64 time=0.047 ms
    ...

## <a name="tls"></a>Securing the docker communication with TLS

If you are
[connecting to the docker daemon with TLS](https://docs.docker.com/articles/https/),
you will probably want to do the same when connecting to the
proxy. That is accomplished by launching the proxy with the same
TLS-related command-line flags as supplied to the docker daemon. For
example, if you have generated your certificates and keys into the
docker host's `/tls` directory, we can launch the proxy with:

    host1$ weave launch-proxy --tlsverify --tlscacert=/tls/ca.pem \
             --tlscert=/tls/server-cert.pem --tlskey=/tls/server-key.pem

The paths to your certificates and key must be provided as absolute
paths which exist on the docker host.

Because the proxy connects to the docker daemon at
`unix:///var/run/docker.sock`, you must ensure that the daemon is
listening there. To do this, you need to pass the `-H
unix:///var/run/docker.sock` option when starting the docker daemon,
in addition to the `-H` options for configuring the TCP listener. See
[the Docker documentation](https://docs.docker.com/articles/basics/#bind-docker-to-another-hostport-or-a-unix-socket)
for an example.

With the proxy running over TLS, we can configure our regular docker
client to use TLS on a per-invocation basis with

    $ docker --tlsverify --tlscacert=ca.pem --tlscert=cert.pem \
         --tlskey=key.pem -H=tcp://host1:12375 version
    ...

or,
[by default](https://docs.docker.com/articles/https/#secure-by-default),
with

    $ mkdir -pv ~/.docker
    $ cp -v {ca,cert,key}.pem ~/.docker
    $ export DOCKER_HOST=tcp://host1:12375 DOCKER_TLS_VERIFY=1
    $ docker version
    ...

which is exactly the same configuration as when connecting to the
docker daemon directly, except that the specified port is the weave
proxy port.

## <a name="troubleshooting"></a>Troubleshooting

The command

    weave status

reports on the current status of various weave components, including
the proxy:

````
...
weave proxy git-03ede29e7ee4
Listen address is :12375
Docker address is unix:///var/run/docker.sock
TLS off
DNS off
IPAM off
````

Information on the operation of the proxy can be obtained from the
weaveproxy container logs with

    docker logs weaveproxy

