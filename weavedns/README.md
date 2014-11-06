# Weave DNS server

The Weave DNS server answers name queries in a Weave network. It is
run per-host, to be supplied as the nameserver for containers on that
host. It is then told about hostnames for the local containers. For
other names it will ask the other weave hosts, or fall back to using
the host's configured name server.

## Using weaveDNS

The weave script command `launch-dns` starts the DNS
container. Subsquently, giving any container a hostname in the domain
`.weave.local` will register it in DNS. For example:

```bash
$ weave launch
$ weave launch-dns 10.1.0.2/16
$ weave run 10.1.1.25/24 -ti -h pingme.weave.local ubuntu
$ shell1=$(weave run 10.1.1.26/24 -ti -h ubuntu.weave.local ubuntu)
$ docker attach $shell1

# ping pingme.weave.local
...
```

The IP address supplied to `weave launch-dns` must not be used by any
other container, and the supplied network must contain all application
networks.

## Doing things more manually

If a weaveDNS container is running, `weave run` will automatically
supply it as the DNS server to the new container. Similarly, both
`weave run` and `weave attach` will register the hostname of the given
container against the given weave network IP address.

In some circumstances, you may not want to use the `weave`
command. You can still take advantage of a running weaveDNS, using the
HTTP API.

### Supplying the DNS server

If you want to use `docker run` to start a container, rather than
`weave run`, you can supply the weaveDNS IP as the `--dns` option to
make it use weaveDNS:

```bash
$ dns_ip=$(docker inspect --format='{{ .NetworkSettings.IPAddress }}' weavedns)
$ shell2=$(docker run --dns=$dns_ip -ti ubuntu)
$ weave attach 10.1.1.27/24 $shell2
```

This isn't very useful unless the container is also attached to the
weave network (as in the last line above).

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
 * The server will currently forget names if it is itself restarted,
   and otherwise not know about containers running when it starts. In
   the future it will look at existing container hostnames upon
   starting up.
 * The server may give unreachable IPs as answers, since it doesn't
   try to filter by reachability. If you use subnets, align your
   hostnames with the subnets.
 * We use UDP multicast to find out about remote names (from Weave DNS
   servers on other hosts); this likely won't scale well beyond a
   certain point T.B.D., so we'll have to come up with another scheme.
