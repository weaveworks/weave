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
* [Adding and removing extra DNS entries](#add-remove)
* [Hot-swapping service containers](#hot-swapping)
* [Retaining DNS entries when containers stop](#retain-stopped)
* [Configuring the domain search path](#domain-search-path)
* [Using a different local domain](#local-domain)
* [Using weaveDNS without `weave run`](#without-run)
* [Troubleshooting](#troubleshooting)
* [Present limitations](#limitations)

## <a name="usage"></a>Using weaveDNS

WeaveDNS is deployed as a set of containers that communicate with each
other over the weave network. One such container needs to be started
on every weave host, by invoking the weave script command
`launch-dns`. Application containers are then instructed to use
weaveDNS as their nameserver by supplying the `--with-dns` option when
starting them; containers so started also automatically register their
container name in the weaveDNS domain. For example:

```bash
$ weave launch
$ weave launch-dns 10.2.254.1/24
$ weave run --with-dns 10.2.1.25/24 -ti --name=pingme ubuntu
$ shell1=$(weave run --with-dns 10.2.1.26/24 -ti --name=ubuntu ubuntu)
$ docker attach $shell1

root@ubuntu:/# ping pingme
...
```

If you start an application container sans `--with-dns` you can still register
it in weaveDNS simply by giving the container a hostname in the
`.weave.local.` domain:

```
$ weave run 10.2.1.25/24 -ti -h pingme.weave.local ubuntu
```

Each weaveDNS container started with `launch-dns` needs to be given
its own, unique, IP address, in a subnet that is common to all
weaveDNS containers and not in use on any of the hosts.

In our example the weaveDNS address is in subnet 10.2.254.0/24 and the
application containers are in subnet 10.2.1.0/24. So, to launch and
use weaveDNS on a second host we would run:

```bash
host2$ weave launch $HOST1
host2$ weave launch-dns 10.2.254.2/24
host2$ shell2=$(weave run --with-dns 10.2.1.36/24 -ti --name=ubuntu2 ubuntu)
host2$ docker attach $shell2

root@ubuntu2:/# ping pingme
...
```

Notice the different IP address in the 10.2.254.0/24 subnet we gave to
`weave launch-dns`, compared to what we supplied on the first host.

WeaveDNS containers can be stopped with `stop-dns`.

## <a name="how-it-works"></a>How it works

The weaveDNS container running on every host acts as the nameserver
for containers on that host. It is told about hostnames for local
containers by the `weave run` command. If a hostname is in the
`.weave.local` domain then weaveDNS records the association of that
name with the container's weave IP address(es).

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
$ sudo DOCKER_BRIDGE=someother weave launch-dns 10.2.254.1/24
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
host2$ weave run --with-dns 10.2.1.35/24 -ti --name=pingme ubuntu
host2$ docker attach $shell2

root@ubuntu2:/# ping -nq -c 1 pingme
PING pingme.weave.local (10.2.1.35) 56(84) bytes of data.
...
root@ubuntu2:/# ping -nq -c 1 pingme
PING pingme.weave.local (10.2.1.25) 56(84) bytes of data.
...
root@ubuntu2:/# ping -nq -c 1 pingme
PING pingme.weave.local (10.2.1.25) 56(84) bytes of data.
...
root@ubuntu2:/# ping -nq -c 1 pingme
PING pingme.weave.local (10.2.1.35) 56(84) bytes of data.
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
$ shell2=(weave run 10.2.1.27/24 -ti ubuntu)
$ weave dns-add 10.2.1.28 $shell2 -h pingme2.weave.local
```

You can also use `dns-add` to add the container's configured hostname
and domain, simply by omitting `-h <fqdn>`.

The inverse operation can be carried out using the `dns-remove` command:

```bash
$ weave dns-remove 10.2.1.27 $shell2
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
$ weave launch-dns 10.2.254.1/24 --watch=false
```

## <a name="ttl"></a>Using a different TTL value

By default, weaveDNS specifies a TTL of 30 seconds in any reply sent to
another peer. Peers will honor the TTL received and cache the answer
until it is considered invalid.

However, you can force a different TTL value by launching weaveDNS with
the `--ttl` argument:

```bash
$ weave launch-dns 10.2.254.1/24 --ttl=10
```

This will shorten the lifespan of answers sent to other peers,
so you will be effectively reducing the probability of them having stale
information, but you will also be increasing their resolution times (as
their cache hit rate will be reduced) and the number of request this
weaveDNS instance will receive.

## <a name="domain-search-path"></a>Configuring the domain search paths

If you don't supply a domain search path (with `--dns-search=`),
`weave run ...` tells a container to look for "bare" hostnames, like
`pingme`, in its own domain. That's why you can just invoke `ping
pingme` above -- since the hostname is `ubuntu.weave.local`, it will
look for `pingme.weave.local`.

If you want to supply other entries for the domain search path,
e.g. if you want containers in different sub-domains to resolve
hostnames across all sub-domains plus some external domains, you need
*also* to supply the `weave.local` domain to retain the above
behaviour.

```bash
weave run --with-dns 10.2.1.4/24 -ti \
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
$ weave launch-dns 10.2.254.1/24 --domain="mycompany.local."
```

The local domain should end with `local.`, since these names are
link-local as per [RFC6762](https://tools.ietf.org/html/rfc6762),
(though this is not strictly necessary).

## <a name="without-run"></a>Using weaveDNS without `weave run`

When weaveDNS is running, both `weave run` and `weave attach` register
the hostname of the given container against the given weave network IP
address. And if you use the `--with-dns` option, `weave run`
automatically supplies the DNS server address to the new container.

In some circumstances, you may not want to use the `weave run` command
to start containers. You can still take advantage of a running
weaveDNS, with some extra manual steps.

### Supplying the DNS server

If you want to start containers with `docker run` rather than `weave
run`, you can supply the docker bridge IP as the `--dns` option to
make it use weaveDNS:

```bash
$ docker_ip=$(docker inspect --format='{{ .NetworkSettings.Gateway }}' weavedns)
$ shell2=$(docker run --dns=$docker_ip -ti ubuntu)
$ weave attach 10.2.1.27/24 $shell2
```

This isn't very useful unless the container is also attached to the
weave network (as in the last line above).

Also note that this means of finding the Docker bridge's IP address
requires a running container (any one would do); another way to find
it is:

```bash
$ docker_ip=$(ip -4 addr show dev docker0 | grep -o 'inet [0-9.]*' | cut -d ' ' -f 2)
```

### Supplying the domain search path

By default, Docker provides containers with a `/etc/resolv.conf` that
matches that for the host. In some circumstances, this may include a
DNS search path, which will break the nice "bare names resolve"
property above.

Therefore, when starting containers with `docker run` instead of
`weave run`, you will usually want to supply a domain search path so
that you can use unqualified hostnames. Use `--dns-search=.` to make
the resolver use the container's domain, or e.g.,
`--dns-search=weave.local` to make it look in `weave.local`.

## <a name="troubleshooting"></a>Troubleshooting

The command

    weave status

reports on the current status of various weave components, including
DNS:

````
weave router git-8f675f15c0b5
...

weave DNS git-8f675f15c0b5
Local domain weave.local.
Listen address :53
mDNS interface &{26 65535 ethwe fa:b6:b1:85:ac:9b up|broadcast|multicast}
Fallback DNS config &{[66.28.0.45 8.8.8.8] [] 53 1 5 2}
Zone database:
710978857a88 10.2.1.26 wiff.weave.local.
e8d85b1dcdb1 10.2.1.27 waff.weave.local.
26bd05f9a0cb 10.2.1.28 ping.weave.local.
1ab0e8b17c39 10.2.1.29 pong.weave.local.
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
