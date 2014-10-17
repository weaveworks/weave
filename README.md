# Weave DNS server

The Weave DNS server answers name queries in a Weave network. It is
run per-host, to be supplied as the nameserver for containers on that
host. It is then told about hostnames for the local containers. For
other names it will ask the other weave hosts, or fall back to using
the host's configured name server.

## Use

The Weave DNS container should be started after the Weave router --
i.e., after running `weave launch` -- and joined to the Weave network.

In general, it should be given an IP in the same subnet as the
router. To watch for container events it must have the docker socker
mounted.

```bash
$ weave launch 10.0.0.1/16
$ weave run 10.0.0.2/16 --name=weavedns -v /var/run/docker.sock:/var/run/docker.sock zettio/weavedns
```

The docker IP for the container can then be provided as the `--dns` value
for other containers:

```bash
$ dns_ip=$(docker inspect --format='{{ .NetworkSettings.IPAddress }}' weavedns)
$ shell1=$(weave run 10.0.1.2/24 --dns=$dns_ip -t ubuntu /bin/bash)
```

To associate a hostname with an IP, use an HTTP PUT to the DNS server:

```bash
$ shell1_ip=$(docker inspect --format='{{ .NetworkSettings.IPAddress }}' $shell1)
$ curl -X PUT "http://$dns_ip:6785/name/$shell1/10.0.1.2?fqdn=shell1.weave&routing_prefix=24&local_ip=$shell1_ip"
```

You should now be able to look up the name:

```bash
$ dig @$dns_ip +short shell1.weave
```

By default, the server will also watch docker events and remove
entries for any containers that die. You can tell it not to by adding
`--watch=false` to the container args:

```bash
$ weave run 10.0.0.2/16 --name=weavedns -v /var/run/docker.sock:/var/run/docker.sock zettio/weavedns --watch=false
```
