---
title: Load Balancing and Fault Resilience with weavedns
layout: default
---



It is permissible to register multiple containers with the same name:
weavedns returns all addresses, in a random order, for each request.
This provides a basic load balancing capability.

Returning to the earlier example, let us start an additional `pingme`
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

**See Also**

 * [How Weave Finds Containers](/site/weave-docker-api/how-works-weavedns.md)
 * [Managing Domains](/site/weavedns/managing-domains-weavedns.md)
 * [Managing Domain Entries](/site/weavedns/managing-entries-weavedns.md)