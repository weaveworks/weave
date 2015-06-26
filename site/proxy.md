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
 * [Securing the docker communication with TLS](#tls)
 * [Launching containers without the proxy](#without-proxy)
 * [Troubleshooting](#troubleshooting)

## <a name="setup"></a>Setup

The proxy sits between the Docker client (command line or API) and the
Docker daemon, intercepting the communication between the two. You can
start it simultaneously with the router and weaveDNS via `launch`:

    host1$ weave launch

or independently via `launch-proxy`:

    host1$ weave launch-router && weave launch-dns && weave launch-proxy

The first form is more convenient, however you can only pass proxy
related configuration arguments to `launch-proxy` so if you need to
modify the default behaviour you will have to use the latter.

By default, the proxy listens on /var/run/weave.sock and port 12375, on
all network interfaces. This can be adjusted with the `-H` argument, e.g.

    host1$ weave launch-proxy -H tcp://127.0.0.1:9999

Multiple -H arguments can be specified. If you are working with a remote
docker daemon, then any firewalls inbetween need to be configured to permit
access to the proxy port.

All docker commands can be run via the proxy, so it is safe to adjust
your `DOCKER_HOST` to point at the proxy. Weave provides a convenient
command for this:

    host1$ eval "$(weave proxy-env)"
    host1$ docker ps
    ...

Alternatively, the proxy host can be set on a per-command basis with

    host1$ docker $(weave proxy-config) ps

The proxy can be stopped independently with

    host1$ weave stop-proxy

or in conjunction with the router and weaveDNS via `stop`.

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

## <a name="ipam"></a>Automatic IP address assignment

If [automatic IP address assignment](ipam.html) is enabled in weave,
which it is by default, then containers started via the proxy will be
automatically assigned an IP address, *without having to specify any
special environment variables or other options*.

    host1$ docker run -ti ubuntu

To use a specific subnet, we pass a `WEAVE_CIDR` to the container, e.g.

    host1$ docker run -ti -e WEAVE_CIDR=net:10.128.0.0/24 ubuntu

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

Containers launched via the proxy will use [weaveDNS](weavedns.html)
automatically if it is running at the point when they are started -
see the [weaveDNS usage](weavedns.html#usage) section for an in-depth
explanation of the behaviour and how to control it.

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
[the Docker documentation](https://docs.docker.com/articles/basics/#bind-docker-to-another-host-port-or-a-unix-socket)
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

## <a name="without-proxy"></a>Launching containers without the proxy

If you cannot or do not want to use the proxy you can launch
containers on the weave network with `weave run`:

    $ weave run -ti ubuntu

The arguments after `run` are passed through to `docker run` so you
can freely specify whichever docker options are appropriate. Once the
container is started, `weave run` attaches it to the weave network, in
this example with an address allocated by IPAM. If you wish you can
specify addresses manually instead:

    $ weave run 10.2.1.1/24 -ti ubuntu

There are some limitations to starting containers with `weave run`:

* containers are always started in the background, i.e. the equivalent
  of always supplying the -d option to docker run
* the --rm option to docker run, for automatically removing containers
  after they stop, is not available
* the weave network interface may not be available immediately on
  container startup.

Finally, there is a `weave start` command which starts existing
containers with `docker start` and attaches them to the weave network.

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

