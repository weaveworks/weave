---
title: Automatic Discovery with WeaveDNS
layout: default
---

# Automatic Discovery with WeaveDNS

The Weave DNS server answers name queries in a Weave network. This
provides a simple way for containers to find each other: just give
them hostnames and tell other containers to connect to those names.
Unlike Docker 'links', this requires no code changes and works across
hosts.

* [Using weaveDNS](#usage)
* [How it works](#how-it-works)
* [Load balancing](#load-balancing)
* [Fault resilience](#fault-resilience)
* [Adding and removing extra DNS entries](#add-remove)
* [Hot-swapping service containers](#hot-swapping)
* [Retaining DNS entries when containers stop](#retain-stopped)
* [Configuring a custom TTL](#ttl)
* [Configuring the domain search path](#domain-search-path)
* [Using a different local domain](#local-domain)
* [Troubleshooting](#troubleshooting)
* [Present limitations](#limitations)

## <a name="usage"></a>Using weaveDNS

WeaveDNS is deployed as a set of containers that communicate with each
other over the weave network. One such container needs to be started
on every weave host, either simultaneously with the router and proxy
via `launch`:

```bash
host1$ weave launch
host1$ eval $(weave proxy-env)
```
or independently via `launch-dns`:

```bash
host1$ weave launch-router && weave launch-dns && weave launch-proxy
host1$ eval $(weave proxy-env)
```

The first form is more convenient, however you can only pass weaveDNS
related configuration arguments to `launch-dns` so if you need to
modify the default behaviour you will have to use the latter.

Application containers will use weaveDNS automatically if it is
running at the point when they are started. They will use it for name
resolution, and will register themselves if they have either a
hostname in the weaveDNS domain (`weave.local` by default) or have
been given an explicit container name:

```bash
host1$ docker run -dti --name=pingme ubuntu
host1$ docker run -ti  --hostname=ubuntu.weave.local ubuntu
root@ubuntu:/# ping pingme
...
```

If both hostname and container name are specified at the same time the
hostname takes precedence; in this circumstance if the hostname is not
in the weaveDNS domain the container will not be registered, but will
still use weaveDNS for resolution.

It is also possible to force or forbid an application container's use
of weaveDNS with the `--with-dns` and `--without-dns` options to
`weave run` and `weave launch-proxy`; these override the runtime
detection of the weaveDNS container.

Each weaveDNS container started with `launch-dns` needs its own unique
IP address in a subnet that is common to all weaveDNS containers. In
the example above we did not specify such an address, so one was
allocated automatically by IPAM from the default subnet; you can
however specify an address in CIDR format manually. In this case you
are responsible for ensuring that the IP addresses specified are
uniquely allocated and not in use by any other container.

Finally, weaveDNS can be stopped independently with

    host1$ weave stop-dns

or in conjunction with the router and proxy via `stop`.

## <a name="how-it-works"></a>How it works

The weaveDNS container running on every host acts as the nameserver
for containers on that host. It learns about hostnames for local
containers from the proxy and from the `weave run` command. If a
hostname is in the `.weave.local` domain then weaveDNS records the
association of that name with the container's weave IP address(es).

When weaveDNS is queried for a name in the `.weave.local` domain, it
first checks its own records. If the name is not found there, it asks
the weaveDNS servers on the other hosts in the weave network.

When weaveDNS is queried for a name in a domain other than
`.weave.local`, it queries the host's configured nameserver,
which is the standard behaviour for Docker containers.

So that containers can connect to a stable and always routable IP
address, weaveDNS publishes its port 53 to the Docker bridge device,
which is assumed to be `docker0`. Some configurations may use a
different Docker bridge device. To supply a different bridge device,
use the environment variable `DOCKER_BRIDGE`, e.g.,

```bash
$ sudo DOCKER_BRIDGE=someother weave launch-dns
```

In the event that weaveDNS is launched in this way, it's important that
other calls to `weave` also specify the bridge device:

```bash
$ sudo DOCKER_BRIDGE=someother weave run --with-dns ...
```

## <a name="load-balancing"></a>Load balancing

It is permissible to register multiple containers with the same name:
weaveDNS picks one address at random on each request. This provides a
basic load balancing capability.

Returning to our earlier example, let us start an additional `pingme`
container, this time on the 2nd host, and then run some ping tests...

```bash
host2$ weave launch
host2$ eval $(weave proxy-env)
host2$ docker run -dti --name=pingme ubuntu

root@ubuntu:/# ping -nq -c 1 pingme
PING pingme.weave.local (10.128.0.2) 56(84) bytes of data.
...
root@ubuntu:/# ping -nq -c 1 pingme
PING pingme.weave.local (10.160.0.1) 56(84) bytes of data.
...
root@ubuntu:/# ping -nq -c 1 pingme
PING pingme.weave.local (10.160.0.1) 56(84) bytes of data.
...
root@ubuntu:/# ping -nq -c 1 pingme
PING pingme.weave.local (10.128.0.2) 56(84) bytes of data.
...
```

Notice how the ping reaches different addresses.

## <a name="fault-resilience"></a>Fault resilience

WeaveDNS removes the addresses of any container that dies. This offers
a simple way to implement redundancy. E.g. if in our example we stop
one of the `pingme` containers and re-run the ping tests, eventually
(within ~30s at most, since that is the weaveDNS [cache expiry time](#ttl)) we
will only be hitting the address of the container that is still alive.


## <a name="add-remove"></a>Adding and removing extra DNS entries

If you want to give the container a name in DNS *other* than its
hostname, you can register it using the `dns-add` command. For example:

```bash
$ C=$(docker run -e WEAVE_CIDR=10.2.1.27/24 -ti ubuntu)
$ weave dns-add 10.2.1.27 $C -h pingme2.weave.local
```

You can also use `dns-add` to add the container's configured hostname
and domain, simply by omitting `-h <fqdn>`.

The inverse operation can be carried out using the `dns-remove` command:

```bash
$ weave dns-remove 10.2.1.27 $C
```

When queried about a name with multiple IPs, weaveDNS returns a random
result from the set of IPs available.

## <a name="hot-swapping"></a>Hot-swapping service containers

If you would like to deploy a new version of a service, keep the old
one running because it has active connections but make all new
requests go to the new version, then you can simply start the new
server container and then [remove](#add-remove) the entry for the old
server container. Later, when all connections to the old server have
terminated, stop the container as normal.

## <a name="retain-stopped"></a>Retaining DNS entries when containers stop

By default, weaveDNS watches docker events and removes entries for any
containers that die. You can tell it not to, by adding `--watch=false`
to the container args:

```bash
$ weave launch-dns --watch=false
```

## <a name="ttl"></a>Configuring a custom TTL

By default, weaveDNS specifies a TTL of 30 seconds in any reply sent to
another peer. Peers will honor the TTL received and cache the answer
until it is considered invalid.

However, you can force a different TTL value by launching weaveDNS with
the `--ttl` argument:

```bash
$ weave launch-dns --ttl=10
```

This will shorten the lifespan of answers sent to other peers,
so you will be effectively reducing the probability of them having stale
information, but you will also be increasing their resolution times (as
their cache hit rate will be reduced) and the number of request this
weaveDNS instance will receive.

## <a name="domain-search-path"></a>Configuring the domain search paths

If you don't supply a domain search path (with `--dns-search=`),
`weave run ...` tells a container to look for "bare" hostnames, like
`pingme`, in its own domain (or in `weave.local` if it has no domain).
That's why you can just invoke `ping pingme` above -- since the
hostname is `ubuntu.weave.local`, it will look for
`pingme.weave.local`.

If you want to supply other entries for the domain search path,
e.g. if you want containers in different sub-domains to resolve
hostnames across all sub-domains plus some external domains, you need
*also* to supply the `weave.local` domain to retain the above
behaviour.

```bash
docker run -ti \
  --dns-search=zone1.weave.local --dns-search=zone2.weave.local \
  --dns-search=corp1.com --dns-search=corp2.com \
  --dns-search=weave.local ubuntu
```

## <a name="local-domain"></a>Using a different local domain

By default, weaveDNS uses `weave.local.` as the domain for names on the
Weave network. In general users do not need to change this domain, but you
can force weaveDNS to use a different domain by launching it
with the `--domain` argument. For example,

```bash
$ weave launch-dns --domain="mycompany.local."
```

The local domain should end with `local.`, since these names are
link-local as per [RFC6762](https://tools.ietf.org/html/rfc6762),
(though this is not strictly necessary).

## <a name="troubleshooting"></a>Troubleshooting

The command

    weave status

reports on the current status of various weave components, including
DNS:

````
...

weave DNS 1.0.0
Listen address :53
Fallback DNS config &{[10.0.2.3] [] 53 1 5 2}

Local domain weave.local.
Interface &{74 65535 ethwe 82:0c:92:84:0e:88 up|broadcast|multicast}
Zone database:
144b75a9b873: pingme.weave.local.[10.160.0.1]/OBS:1
74a5510a91ad: ubuntu.weave.local.[10.160.0.2]
weave:remote: pingme.weave.local.[10.128.0.2]/TTL:30

...
````

The first section covers the router; see the [troubleshooting
guide](troubleshooting.html#status-report) for more detail.

The second section is pertinent to weaveDNS, and includes:

* The local domain suffix which is being served
* The address on which the DNS server is listening
* The interface being used for multicast DNS
* The fallback DNS which will be used to resolve non local names
* The names known to the local weaveDNS server. Each entry comprises
  the container ID, IP address and its fully qualified domain name

Information on the processing of queries, and the general operation of
weaveDNS, can be obtained from the container logs with

    docker logs weavedns

## <a name="limitations"></a>Present limitations

 * The server will not know about restarted containers, but if you
   re-attach a restarted container to the weave network, it will be
   re-registered with weaveDNS.
 * The server may give unreachable IPs as answers, since it doesn't
   try to filter by reachability. If you use subnets, align your
   hostnames with the subnets.
 * We use UDP multicast to find out about remote names (from weaveDNS
   servers on other hosts); this likely won't scale well beyond a
   certain point T.B.D.
