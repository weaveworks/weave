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

    host1$ weave launch-proxy -H tcp://127.0.0.1:9999

If you are working with a remote docker daemon, then any firewalls
inbetween need to be configured to permit access to the proxy port.

All docker commands can be run via the proxy, so it is safe to adjust
your `DOCKER_HOST` to point at the proxy. Weave provides a convenient
command for this:

    host1$ eval "$(weave proxy-env)"
    host1$ docker ps
    ...

Alternatively, the proxy host can be set on a per-command basis with

    host1$ docker $(weave proxy-config) ps

The proxy can be stopped with

    host1$ weave stop-proxy

If you set your `DOCKER_HOST` to point at the proxy, remember to
revert to the original setting.

## <a name="usage"></a>Usage

When containers are created via the weave proxy, their entrypoint will
be modified to wait for the weave network interface to become
available. When they are started via the weave proxy, containers will
be [automatically assigned IP addresses](#ipam) and connected to the
weave network. We can create and start a container via the weave proxy
with

    host1$ docker run -ti ubuntu

or, equivalently with

    host1$ docker create -ti ubuntu
    5ef831df61d50a1a49272357155a976595e7268e590f0a2c75693337b14e1382
    host1$ docker start 5ef831df61d50a1a49272357155a976595e7268e590f0a2c75693337b14e1382

Specific IP addresses and networks can be supplied in the `WEAVE_CIDR`
environment variable, e.g.

    host1$ docker run -e WEAVE_CIDR=10.2.1.1/24 -ti ubuntu

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
which it is by default, then containers started via the proxy will be
automatically assigned an IP address, *without having to specify any
special environment variables or other options*.

    host1$ docker run -ti ubuntu

To use a specific IP, we pass a `WEAVE_CIDR` to the container, e.g.

    host1$ docker run -ti -e WEAVE_CIDR=10.2.1.1/24 ubuntu

To start a container without connecting it to the weave network, pass
`WEAVE_CIDR=none`, e.g.

    host1$ docker run -ti -e WEAVE_CIDR=none ubuntu

If you do not want IPAM to be used by default, the proxy needs to be
passed the `--no-default-ipam` flag, e.g.

    host1$ docker launch-proxy --no-default-ipam

In this configuration, containers with no `WEAVE_CIDR` environment
variable will not be connected to the weave network. Containers
started with a `WEAVE_CIDR` environment variable are handled as
before. To automatically assign an address in this mode, we start the
container with a blank `WEAVE_CIDR`, e.g.

    host1$ docker run -ti -e WEAVE_CIDR= ubuntu

## <a name="dns"></a>Automatic discovery

Containers started via the proxy are automatically registered in
[weaveDNS](weavedns.html) if they have a hostname in the weaveDNS
domain (usually `.weave.local`). In order for containers to be able to
look up such names, their DNS resolver needs to be configured to point
at weaveDNS. This will be done automatically by the proxy while
weaveDNS is running. To override this behaviour launch the proxy with
either `--with-dns` or `--without-dns`, which will force the proxy to
always/never set the resolver to weaveDNS. If there is no hostname
provided, the container will be registered in weaveDNS using its
container name. Otherwise, if there is no container name, and no
hostname (or a hostname outside the weaveDNS domain), the container
will not be registered in weaveDNS.

## <a name="multi-host"></a>Multi-host example

Here's a complete example of using weave proxies configured with
[automatic IP address assignment](#ipam) and
[automatic discovery](#dns) to start containers on two hosts such that
one can reach the other by name.

First, let us start weave, weaveDNS and the proxy, and set DOCKER_HOST
to point at the latter:

    host1$ weave launch
    host1$ weave launch-dns
    host1$ weave launch-proxy
    host1$ eval "$(weave proxy-env)"

    host2$ weave launch host1
    host2$ weave launch-dns
    host2$ weave launch-proxy
    host2$ eval "$(weave proxy-env)"

Now let us start a named container on one host, and ping it, by name,
from a container on the other host.

    host1$ docker run --name=pingme -dti ubuntu

    host2$ docker run -ti ubuntu ping pingme
    PING pingme.weave.local (10.128.0.2) 56(84) bytes of data.
    64 bytes from pingme.weave.local (10.128.0.2): icmp_seq=1 ttl=64 time=0.047 ms
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
    $ eval "$(weave proxy-env)"
    $ export DOCKER_TLS_VERIFY=1
    $ docker version
    ...

which is exactly the same configuration as when connecting to the
docker daemon directly, except that the specified port is the weave
proxy port.

## <a name="troubleshooting"></a>Troubleshooting

The command

    weave status

reports on the current status of various weave components, including
the proxy, if it is running:

````
...
weave proxy is running
````

Information on the operation of the proxy can be obtained from the
weaveproxy container logs with

    docker logs weaveproxy

