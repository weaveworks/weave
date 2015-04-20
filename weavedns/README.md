# Weave DNS server

The Weave DNS server answers name queries in a Weave network. This
provides a simple way for containers to find each other: just give
them hostnames and tell other containers to connect to those names.
Unlike Docker 'links', this requires no code changes and works across
hosts.

## Using weaveDNS

WeaveDNS is deployed as a set of containers that communicate with each
other over the weave network. One such container needs to be started
on every weave host, by invoking the weave script command
`launch-dns`. Application containers are then instructed to use
WeaveDNS as their nameserver by supplying the `--with-dns` option when
starting them. Giving any container a hostname in the `.weave.local`
domain registers it in weaveDNS.  For example:

```bash
$ weave launch
$ weave launch-dns 10.2.254.1/24
$ weave run 10.2.1.25/24 -ti -h pingme.weave.local ubuntu
$ shell1=$(weave run --with-dns 10.2.1.26/24 -ti -h ubuntu.weave.local ubuntu)
$ docker attach $shell1

# ping pingme
...
```

Each weaveDNS container started with `launch-dns` needs to be given
its own, unique, IP address, in a subnet that is a) common to all
weaveDNS containers, b) disjoint from the application subnets, and c)
not in use on any of the hosts. In our example the weaveDNS address is
in subnet 10.2.254.0/24 and the application containers are in
subnet 10.2.1.0/24.

WeaveDNS containers can be stopped with `stop-dns`.

## How it works

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

## Domain search paths

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

## Doing things more manually

If you use the `--with-dns` option, `weave run` automatically supplies
the DNS server address to the new container. And both `weave run` and
`weave attach` register the hostname of the given container against
the given weave network IP address.

In some circumstances, you may not want to use the `weave`
command. You can still take advantage of a running weaveDNS, with some
extra manual steps.

### Using a different docker bridge

So that containers can connect to a stable and always routable IP
address, weaveDNS publishes its port 53 to the Docker bridge device,
which is assumed to be `docker0`.

Some configurations may use a different Docker bridge device. To
supply a different bridge device, use the environment variable
`DOCKER_BRIDGE`, e.g.,

```bash
$ sudo DOCKER_BRIDGE=someother weave launch-dns 10.2.254.1/24
```

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

### Using a different local domain

By default, WeaveDNS uses `weave.local.` as the domain for names on the
Weave network. In general users do not need to change this domain, but you
can force WeaveDNS to use a different domain by launching it
with the `--domain` argument. For example,

```bash
$ weave launch-dns 10.2.254.1/24 --domain="mycompany.local."
```

The local domain should end with `local.`, since these names are
link-local as per [RFC6762](https://tools.ietf.org/html/rfc6762),
(though this is not strictly neccessary).

### Adding containers to DNS

If DNS is started after you've attached a container to the weave
network, or you want to give the container a name in DNS *other* than
its hostname, you can register it using the HTTP API:

```bash
$ docker start $shell2
$ curl -X PUT "http://$dns_ip:6785/name/$shell2/10.2.1.27" -d fqdn=shell2.weave.local
```

### Registering multiple containers with the same name

This is supported; weaveDNS picks one address to return when you ask
for the name. Since weaveDNS removes any container that dies, this is
a simple way to implement redundancy.  In the current implementation
it does not attempt to do load-balancing.

### Replacing one container with another at the same name

If you would like to deploy a new version of a service, keep the old one running because it has active connections but make all new requests go to the new version, then you can simply start the new server container and then [unregister](https://github.com/weaveworks/weave/tree/master/weavedns#unregistering) the old one from DNS. And finally, when all connections to the old server have terminated, stop the container as normal.

### Not watching docker events

By default, weaveDNS watches docker events and removes entries for any
containers that die. You can tell it not to, by adding `--watch=false`
to the container args:

```bash
$ weave launch-dns 10.2.254.1/24 --watch=false
```

### Unregistering

You can manually delete entries for a host, by poking weaveDNS's HTTP
API with e.g., `curl`:

```bash
$ docker stop $shell2
$ dns_ip=$(docker inspect --format='{{ .NetworkSettings.IPAddress }}' weavedns)
$ curl -X DELETE "http://$dns_ip:6785/name/$shell2/10.2.1.27"
```

## Troubleshooting

The command

    weave status

reports on the current status of the weave router and DNS:

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

The first section covers the router; see the troubleshooting guide in
the main documentation for more detail.

The second section is pertinent to weaveDNS, and includes:

* The local domain suffix which is being served
* The address on which the DNS server is listening
* The interface being used for multicast DNS
* The fallback DNS which will be used to resolve non local names
* The names known to the local weaveDNS server. Each entry comprises the container ID, IP address and its fully qualified domain name

## Present limitations

 * The server will not know about restarted containers, but if you
   re-attach a restarted container to the weave network, it will be
   re-registered with weaveDNS.
 * The server may give unreachable IPs as answers, since it doesn't
   try to filter by reachability. If you use subnets, align your
   hostnames with the subnets.
 * We use UDP multicast to find out about remote names (from weaveDNS
   servers on other hosts); this likely won't scale well beyond a
   certain point T.B.D., so we'll have to come up with another scheme.
