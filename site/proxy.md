---
title: Weave Docker API Proxy
layout: default
---

# Weave Docker API Proxy

The Docker API proxy automatically attaches containers to the weave
network when they are started using the ordinary Docker
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

    host1$ weave launch-router && weave launch-proxy

The first form is more convenient, however you can only pass proxy
related configuration arguments to `launch-proxy` so if you need to
modify the default behaviour you will have to use the latter.

By default, the proxy decides where to listen based on how the
launching client connects to docker. If the launching client connected
over a unix socket, the proxy will listen on /var/run/weave/weave.sock. If
the launching client connected over TCP, the proxy will listen on port
12375, on all network interfaces. This can be adjusted with the `-H`
argument, e.g.

    host1$ weave launch-proxy -H tcp://127.0.0.1:9999

If no TLS or listening interfaces are set, TLS will be autoconfigured
based on the docker daemon's settings, and listening interfaces will
be autoconfigured based on your docker client's settings.

Multiple `-H` arguments can be specified. If you are working with a
remote docker daemon, then any firewalls inbetween need to be
configured to permit access to the proxy port.

All docker commands can be run via the proxy, so it is safe to adjust
your `DOCKER_HOST` to point at the proxy. Weave provides a convenient
command for this:

    host1$ eval $(weave env)
    host1$ docker ps
    ...

The prior settings can be restored with

    host1$ eval $(weave env --restore)

Alternatively, the proxy host can be set on a per-command basis with

    host1$ docker $(weave config) ps

The proxy can be stopped independently with

    host1$ weave stop-proxy

or in conjunction with the router and weaveDNS via `stop`.

If you set your `DOCKER_HOST` to point at the proxy, you should revert
to the original settings prior to stopping the proxy.


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

The docker NetworkSettings (including IP address, MacAddress, and
IPPrefixLen), will still be returned by `docker inspect`. If you want
`docker inspect` to return the weave NetworkSettings instead, then the
proxy must be launced with the `--rewrite-inspect` flag. This will
only substitute in the weave network settings when the container has a
weave IP. If a container has more than one weave IP, the inspect call
will only include one of them.

    host1$ weave launch-router && weave launch-proxy --rewrite-inspect

## <a name="ipam"></a>Automatic IP address assignment

If [automatic IP address assignment](ipam.html) is enabled in weave,
which it is by default, then containers started via the proxy will be
automatically assigned an IP address, *without having to specify any
special environment variables or other options*.

    host1$ docker run -ti ubuntu

To use a specific subnet, we pass a `WEAVE_CIDR` to the container, e.g.

    host1$ docker run -ti -e WEAVE_CIDR=net:10.32.2.0/24 ubuntu

To start a container without connecting it to the weave network, pass
`WEAVE_CIDR=none`, e.g.

    host1$ docker run -ti -e WEAVE_CIDR=none ubuntu

If you do not want an IP to be assigned by default, the proxy needs to
be passed the `--no-default-ipalloc` flag, e.g.,

    host1$ weave launch-proxy --no-default-ipalloc

In this configuration, containers with no `WEAVE_CIDR` environment
variable will not be connected to the weave network. Containers
started with a `WEAVE_CIDR` environment variable are handled as
before. To automatically assign an address in this mode, we start the
container with a blank `WEAVE_CIDR`, e.g.

    host1$ docker run -ti -e WEAVE_CIDR="" ubuntu

## <a name="etchosts"></a>Name resolution via `/etc/hosts`

When starting weave-enabled containers, the proxy will automatically
replace the container's `/etc/hosts` file, and disable Docker's control
over it. The new file contains an entry for the container's hostname
and weave IP address, as well as additional entries that have been
specified with `--add-host` parameters. This ensures that

- name resolution of the container's hostname, e.g. via `hostname -i`,
returns the weave IP address. This is required for many cluster-aware
applications to work.
- unqualified names get resolved via DNS, i.e. typically via weaveDNS
to weave IP addresses. This is required so that in a typical setup
one can simply "ping <container-name>", i.e. without having to
specify a `.weave.local` suffix.

In case you prefer to keep `/etc/hosts` under Docker's control (for
example, because you need the hostname to resolve to the Docker-assigned
IP instead of the weave IP, or you require name resolution for
Docker-managed networks), the proxy must be launched with the
`--no-rewrite-hosts` flag.

    host1$ weave launch-router && weave launch-proxy --no-rewrite-hosts

## <a name="dns"></a>Automatic discovery

Containers launched via the proxy will use [weaveDNS](weavedns.html)
automatically if it is running at the point when they are started -
see the [weaveDNS usage](weavedns.html#usage) section for an in-depth
explanation of the behaviour and how to control it.

Typically, the proxy will pass on container names as-is to [weaveDNS](weavedns.html)
for registration. However, there are situations in which the final container
name is out of the user's control (e.g. when using Docker orchestrators which
append control/namespacing identifiers to the original container names).

For those situations, the proxy provides a few flags: `--hostname-from-label
<labelkey>`, `--hostname-match <regexp>` and `--hostname-replacement
<replacement>`. When launching a container, the hostname is initialized to the
value of the container label with key `<labelkey>`, if `<labelkey>` wasn't
provided, the container name is used. Additionally, the hostname is matched
against regular expression `<regexp>`. Then, based on that match,
`<replacement>` will be used to obtainer the final hostname, which will
ultimately be handed over to weaveDNS for registration.

For instance, we can launch the proxy using all three flags

    host1$ weave launch-router && weave launch-proxy --hostname-from-label hostname-label --hostname-match '^aws-[0-9]+-(.*)$' --hostname-replacement 'my-app-$1'
    host1$ eval $(weave env)

Note how regexp substitution groups should be prepended with a dollar sign
(e.g. `$1`). For further details on the regular expression syntax please see
[Google's re2 documentation](https://github.com/google/re2/wiki/Syntax).


Then, running a container named `aws-12798186823-foo` without labels will lead
to weaveDNS registering hostname `my-app-foo` and not `aws-12798186823-foo`.

    host1$ docker run -ti --name=aws-12798186823-foo ubuntu ping my-app-foo
    PING my-app-foo.weave.local (10.32.0.2) 56(84) bytes of data.
    64 bytes from my-app-foo.weave.local (10.32.0.2): icmp_seq=1 ttl=64 time=0.027 ms
    64 bytes from my-app-foo.weave.local (10.32.0.2): icmp_seq=2 ttl=64 time=0.067 ms

Also, running a container named `foo` with label
`hostname-label=aws-12798186823-foo` leads to the same hostname registration.

    host1$ docker run -ti --name=foo --label=hostname-label=aws-12798186823-foo ubuntu ping my-app-foo
    PING my-app-foo.weave.local (10.32.0.2) 56(84) bytes of data.
    64 bytes from my-app-foo.weave.local (10.32.0.2): icmp_seq=1 ttl=64 time=0.031 ms
    64 bytes from my-app-foo.weave.local (10.32.0.2): icmp_seq=2 ttl=64 time=0.042 ms

This is because, as we explained above, when providing `--hostname-from-label`
to the proxy, the specified label has precedence over the container's name.

## <a name="tls"></a>Securing the docker communication with TLS

If you are [connecting to the docker daemon with
TLS](https://docs.docker.com/articles/https/), you will probably want
to do the same when connecting to the proxy. The proxy will
automatically detect the docker daemon's TLS configuration, and
attempt to duplicate it. In the standard auto-detection case you will
be able to launch a TLS-enabled proxy with:

    host1$ weave launch-proxy

To disable auto-detection of TLS configuration, you can either pass
the `--no-detect-tls` flag, or manually configure the proxy's TLS with
the same TLS-related command-line flags as supplied to the docker
daemon. For example, if you have generated your certificates and keys
into the docker host's `/tls` directory, we can launch the proxy with:

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
    $ eval $(weave env)
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
this example with an automatically allocated IP. If you wish you can
specify addresses manually instead:

    $ weave run 10.2.1.1/24 -ti ubuntu

`weave run` will rewrite `/etc/hosts` in the same way
[the proxy does](#etchosts). In case you prefer to keep
the original file, you must specify `--no-rewrite-hosts` when running
the container:

    $ weave run --no-rewrite-hosts 10.2.1.1/24 -ti ubuntu

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

