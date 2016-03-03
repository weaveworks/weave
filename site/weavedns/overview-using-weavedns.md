---
title: Using Weavedns
layout: default
---



The Weave DNS server answers name queries on a Weave network and provides a simple way for containers to find each other. Just give
the containers hostnames and then tell other containers to connect to those names.
Unlike Docker 'links', this requires no code changes and it works across
hosts.

Weavedns is deployed as an embedded service within the Weave router.
The service is automatically started when the router is launched:

```bash
host1$ weave launch
host1$ eval $(weave env)
```

Weavedns related configuration arguments can be passed to `launch`.

Application containers use weavedns automatically if it is
running when they are started. They use it for name
resolution, and will register themselves if they either have a
hostname in the weavedns domain (`weave.local` by default) or are given an explicit container name:

```bash
host1$ docker run -dti --name=pingme ubuntu
host1$ docker run -ti  --hostname=ubuntu.weave.local ubuntu
root@ubuntu:/# ping pingme
...
```

>> **Note** If both hostname and container name are specified at
the same time, the hostname takes precedence. In this circumstance, if
the hostname is not in the weavedns domain, the container is *not*
registered, but it will still use weavedns for resolution.

To disable an application container's use of weavedns, add the
`--without-dns` option to `weave run` or `weave launch-proxy`.


**See Also**

 * [How Weave Finds Containers](/site/weave-docker-api/how-works-weavedns.md)
 * [Load Balancing and Fault Resilience with weavedns](/site/weave-docker-api/load-balance-fault-weavedns.md)
 