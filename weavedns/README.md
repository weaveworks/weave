# Weave DNS server

The Weave DNS server answers name queries in a Weave network. It is
run per-host, to be supplied as the nameserver for containers on that
host. It is then told about hostnames for the local containers. For
other names it will ask the other weave hosts, or fall back to using
the host's configured name server.

## Using weaveDNS

The weave script command `launch-dns` starts the DNS container, and
then you use the `--with-dns` option on containers you wish to use it.
Subsquently, giving any container a hostname in the domain
`.weave.local` will register it in DNS. For example:

```bash
$ weave launch
$ weave launch-dns 10.1.0.2/16
$ weave run 10.1.1.25/24 -ti -h pingme.weave.local ubuntu
$ shell1=$(weave run --with-dns 10.1.1.26/24 -ti -h ubuntu.weave.local ubuntu)
$ docker attach $shell1

The DNS container can be stopped with `stop-dns`.

# ping pingme
...
```

The weave IP address supplied to `weave launch-dns` must not be used
by any other container, and the supplied network must contain all
application networks.

## Domain search paths

If you don't supply a domain search path (with `--dns-search=`),
`weave run ...` will tell a container to look for "bare" hostnames,
like `pingme`, in its own domain. That's why you can just say `ping
pingme` above -- since the hostname is `ubuntu.weave.local`, it will
look for `pingme.weave.local`.

If you want to supply other entries for the domain search path,
e.g. if you want containers in different sub-domains to resolve
hostnames across all sub-domains plus some external domains, you will
need *also* to supply the `weave.local` domain to retain the above
behaviour.

```bash
weave run --with-dns 10.1.1.4/24 -ti \
  --dns-search=zone1.weave.local --dns-search=zone2.weave.local \
  --dns-search=corp1.com --dns-search=corp2.com \
  --dns-search=weave.local ubuntu
```

## Doing things more manually

If you use the `--with-dns` option, `weave run` will automatically
supply the DNS server address to the new container. And both
`weave run` and `weave attach` will register the hostname of the given
container against the given weave network IP address.

In some circumstances, you may not want to use the `weave`
command. You can still take advantage of a running weaveDNS, with some
extra manual steps.

### Using a different docker bridge

So that containers can connect to a stable and always routable IP
address, weaveDNS publishes its port 53 to the Docker bridge device,
which is assumed to be `docker0`.

Some configurations will use a different Docker bridge device. To
supply a different bridge device, use the environment variable
`DOCKER_BRIDGE`, e.g.,

```bash
$ sudo DOCKER_BRIDGE=someother weave launch-dns 10.0.1.2/16
```

### Supplying the DNS server

If you want to start containers with `docker run` rather than `weave
run`, you can supply the docker bridge IP as the `--dns` option to
make it use weaveDNS:

```bash
$ docker_ip=$(docker inspect --format='{{ .NetworkSettings.Gateway }}' weavedns)
$ shell2=$(docker run --dns=$docker_ip -ti ubuntu)
$ weave attach 10.1.1.27/24 $shell2
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

By default, Docker will provide containers with a `/etc/resolv.conf`
that matches that for the host. In some circumstances, this may
include a DNS search path, which will break the nice "bare names
resolve" property above.

Therefore, when starting containers with `docker run` instead of
`weave run`, you will usually want to supply a domain search path so
that you can use unqualified hostnames. Use `--dns-search=.` to make
the resolver use the container's domain, or e.g.,
`--dns-search=weave.local` to make it look in `weave.local`.

### Adding containers to DNS

If DNS is started after you've attached a container to the weave
network, or you want to give the container a name in DNS *other* than
its hostname, you can register it using the HTTP API:

```bash
$ docker start $shell2
$ shell2_ip=$(docker inspect --format='{{ .NetworkSettings.IPAddress }}' $shell2)
$ curl -X PUT "http://$dns_ip:6785/name/$shell2/10.1.1.27" -d local_ip=$shell2_ip -d fqdn=shell2.weave.local
```

### Not watching docker events

By default, the server will watch docker events and remove entries for
any containers that die. You can tell it not to, by adding
`--watch=false` to the container args:

```bash
$ weave launch-dns 10.1.0.2/16 --watch=false
```

You can manually delete entries for a host, by poking weaveDNS's HTTP
API with e.g., `curl`:

```bash
$ docker stop $shell2
$ dns_ip=$(docker inspect --format='{{ .NetworkSettings.IPAddress }}' weavedns)
$ curl -X DELETE "http://$dns_ip:6785/name/$shell2/10.1.1.27"
```

## Present limitations

 * The server will not know about restarted containers, but if you
   re-attach a restarted container to the weave network, it will be
   re-registered with weaveDNS.
 * The server may give unreachable IPs as answers, since it doesn't
   try to filter by reachability. If you use subnets, align your
   hostnames with the subnets.
 * We use UDP multicast to find out about remote names (from Weave DNS
   servers on other hosts); this likely won't scale well beyond a
   certain point T.B.D., so we'll have to come up with another scheme.
