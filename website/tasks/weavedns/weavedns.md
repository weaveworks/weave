---
title: Discovering Containers with WeaveDNS
menu_order: 5
search_type: Documentation
---



WeaveDNS is a DNS server that answers name queries on a Weave network
and provides a simple way for containers to find each other. Just give
the containers hostnames and then tell other containers to connect to
those names.  Unlike Docker 'links', this requires no code changes and
it works across hosts.

WeaveDNS is deployed as an embedded service within the Weave router.
The service is automatically started when the router is launched:

```
host1$ weave launch
host1$ eval $(weave env)
```

WeaveDNS related configuration arguments can be passed to `launch`.

Application containers use weaveDNS automatically if it is
running when they are started. They use it for name
resolution, and will register themselves if they either have a
hostname in the weaveDNS domain (`weave.local` by default) or are given an explicit container name:

```
host1$ docker run -dti --name=pingme alpine:latest
host1$ docker run -ti  --hostname=ubuntu.weave.local alpine:latest
root@ubuntu:/# ping pingme
...
```

Moreover, weaveDNS always register all network aliases (--network-alias option to docker run).

```
host1$ docker run --network weave --network-alias pingme --network-alias pingme2 -dti alpine:latest
host1$ docker run --network weave --hostname=ubuntu.weave.local -ti alpine:latest
root@ubuntu:/# ping pingme
...
root@ubuntu:/# ping pingme2
...
```

> **Note** If both hostname and container name are specified at
the same time, the hostname takes precedence. In this circumstance, if
the hostname is not in the weaveDNS domain, the container is *not*
registered, but it will still use weaveDNS for resolution.

By default, weaveDNS will listen on port 53 on the address of the
Docker bridge. To make it listen on a different address or port use
`weave launch --dns-listen-address <address>:<port>`

To disable an application container's use of weaveDNS, add the
`--without-dns` option to `weave launch`. To
disable weaveDNS itself, launch weave with the `--no-dns` option.

> **Note** WeaveDNS is not part of the Weave Net Kubernetes add-on.
    Kubernetes has its own DNS service, integrated with Kubernetes
    Services, and WeaveDNS does not duplicate that functionality.

**See Also**

 * [Integrating Docker via the API Proxy]({{ '/tasks/weave-docker-api/weave-docker-api' | relative_url }})
 * [Load Balancing and Fault Resilience with WeaveDNS]({{ '/tasks/weavedns/load-balance-fault-weavedns' | relative_url }})
