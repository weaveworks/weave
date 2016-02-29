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
* [Resolve weaveDNS entries from host](#resolve-weavedns-entries-from-host)
* [Hot-swapping service containers](#hot-swapping)
* [Configuring a custom TTL](#ttl)
* [Configuring the domain search path](#domain-search-path)
* [Using a different local domain](#local-domain)
* [Troubleshooting](#troubleshooting)
* [Present limitations](#limitations)

## <a name="usage"></a>Using weaveDNS

WeaveDNS is deployed as an embedded service within the Weave router.
The service is automatically started when the router is launched:

```bash
host1$ weave launch
host1$ eval $(weave env)
```

WeaveDNS related configuration arguments can be passed to `launch`.

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

> **Please note** if both hostname and container name are specified at
> the same time the hostname takes precedence; in this circumstance if
> the hostname is not in the weaveDNS domain the container will *not*
> be registered, but will still use weaveDNS for resolution.

To disable application containers' use of weaveDNS, add the
`--without-dns` option to `weave run` or `weave launch-proxy`.

## <a name="how-it-works"></a>How it works

The weaveDNS service running on every host acts as the nameserver for
containers on that host. It learns about hostnames for local containers
from the proxy and from the `weave run` command.  If a hostname is in
the `.weave.local` domain then weaveDNS records the association of that
name with the container's weave IP address(es) in its in-memory
database, and broadcasts the association to other weave peers in the
cluster.

When weaveDNS is queried for a name in the `.weave.local` domain, it
looks up the hostname its in memory database and responds with the IPs
of all containers for that hostname across the entire cluster.

WeaveDNS returns IP addresses in a random order to facilitate basic
load balancing and failure tolerance. Most client side resolvers sort
the returned addresses based on reachability, placing local addresses
at the top of the list (see [RFC 3484](https://www.ietf.org/rfc/rfc3484.txt)).
For example, if there is container with the desired hostname on the local
machine, the application will receive that container's IP address.
Otherwise, the application will receive the IP address of a random
container with the desired hostname.

When weaveDNS is queried for a name in a domain other than
`.weave.local`, it queries the host's configured nameserver, which is
the standard behaviour for Docker containers.

So that containers can connect to a stable and always routable IP
address, weaveDNS listens on port 53 to the Docker bridge device, which
is assumed to be `docker0`.  Some configurations may use a different
Docker bridge device. To supply a different bridge device, use the
environment variable `DOCKER_BRIDGE`, e.g.,

```bash
$ sudo DOCKER_BRIDGE=someother weave launch
```

In the event that weaveDNS is launched in this way, it's important that
other calls to `weave` also specify the bridge device:

```bash
$ sudo DOCKER_BRIDGE=someother weave run ...
```

## <a name="load-balancing"></a>Load balancing

It is permissible to register multiple containers with the same name:
weaveDNS returns all addresses, in a random order, for each request.
This provides a basic load balancing capability.

Returning to our earlier example, let us start an additional `pingme`
container, this time on the 2nd host, and then run some ping tests...

```bash
host2$ weave launch
host2$ eval $(weave env)
host2$ docker run -dti --name=pingme ubuntu

root@ubuntu:/# ping -nq -c 1 pingme
PING pingme.weave.local (10.32.0.2) 56(84) bytes of data.
...
root@ubuntu:/# ping -nq -c 1 pingme
PING pingme.weave.local (10.40.0.1) 56(84) bytes of data.
...
root@ubuntu:/# ping -nq -c 1 pingme
PING pingme.weave.local (10.40.0.1) 56(84) bytes of data.
...
root@ubuntu:/# ping -nq -c 1 pingme
PING pingme.weave.local (10.32.0.2) 56(84) bytes of data.
...
```

Notice how the ping reaches different addresses.

## <a name="fault-resilience"></a>Fault resilience

WeaveDNS removes the addresses of any container that dies. This offers
a simple way to implement redundancy. E.g. if in our example we stop
one of the `pingme` containers and re-run the ping tests, eventually
(within ~30s at most, since that is the weaveDNS
[cache expiry time](#ttl)) we will only be hitting the address of the
container that is still alive.

## <a name="add-remove"></a>Adding and removing extra DNS entries

If you want to give the container a name in DNS *other* than its
hostname, you can register it using the `dns-add` command. For example:

```bash
$ C=$(docker run -ti ubuntu)
$ weave dns-add $C -h pingme2.weave.local
```

You can also use `dns-add` to add the container's configured hostname
and domain simply by omitting `-h <fqdn>`, or specify additional IP
addresses to be registered against the container's hostname e.g.
`weave dns-add 10.2.1.27 $C`.

The inverse operation can be carried out using the `dns-remove`
command:

```bash
$ weave dns-remove $C
```

By omitting the container name it is possible to add/remove DNS
records that associate names in the weaveDNS domain with IP addresses
that do not belong to containers, e.g. non-weave addresses of external
services:

```bash
$ weave dns-add 192.128.16.45 -h db.weave.local
```

Note that such records get removed when stopping the weave peer on
which they were added.

## <a name="resolve-weavedns-entries-from-host"></a>Resolve weaveDNS entries from host

You can resolve entries from any host running weaveDNS with `weave
dns-lookup`:

    host1$ weave dns-lookup pingme
    10.40.0.1

## <a name="hot-swapping"></a>Hot-swapping service containers

If you would like to deploy a new version of a service, keep the old
one running because it has active connections but make all new
requests go to the new version, then you can simply start the new
server container and then [remove](#add-remove) the entry for the old
server container. Later, when all connections to the old server have
terminated, stop the container as normal.


## <a name="ttl"></a>Configuring a custom TTL

By default, weaveDNS specifies a TTL of 30 seconds in responses to DNS
requests.  However, you can force a different TTL value by launching
weave with the `--dns-ttl` argument:

```bash
$ weave launch --dns-ttl=10
```

This will shorten the lifespan of answers sent to clients, so you will
be effectively reducing the probability of them having stale
information, but you will also be increasing the number of request this
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
Weave network. In general users do not need to change this domain, but
you can force weaveDNS to use a different domain by launching it with
the `--dns-domain` argument. For example,

```bash
$ weave launch --dns-domain="mycompany.local."
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

       Service: dns
        Domain: weave.local.
      Upstream: 8.8.8.8, 8.8.4.4
           TTL: 1
       Entries: 9

...
````

The first section covers the router; see the [troubleshooting
guide](troubleshooting.html#weave-status) for more detail.

The 'Service: dns' section is pertinent to weaveDNS, and includes:

* The local domain suffix which is being served
* The list of upstream servers used for resolving names not in the local domain
* The response ttl
* The total number of entries

You may also use `weave status dns` to obtain a [complete
dump](troubleshooting.html#weave-status-dns) of all DNS registrations.

Information on the processing of queries, and the general operation of
weaveDNS, can be obtained from the container logs with

    docker logs weave

## <a name="limitations"></a>Present limitations

 * The server will not know about restarted containers, but if you
   re-attach a restarted container to the weave network, it will be
   re-registered with weaveDNS.
 * The server may give unreachable IPs as answers, since it doesn't
   try to filter by reachability. If you use subnets, align your
   hostnames with the subnets.
