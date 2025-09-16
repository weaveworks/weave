---
title: Manually Specifying the IP Address of a Container
menu_order: 20
search_type: Documentation
---

Containers are automatically allocated an IP address that is unique across the Weave network. You can see which address was allocated by running, [`weave ps`]({{ '/troubleshooting#weave-status' | relative_url }}):

    host1$ weave ps a1
    a7aee7233393 7a:44:d3:11:10:70 10.32.0.2/12

Weave Net detects when a container has exited and releases its allocated addresses so they can be re-used by the network.

See the [Automatic IP Address Management]({{ '/tasks/ipam/ipam' | relative_url }}) and also an explanation of [the basics of IP addressing]({{ '/concepts/ip-addresses' | relative_url }}) for further details.

Instead of allowing Weave Net to allocate IP addresses automatically (using IPAM), there may be instances where you need to control a particular container or a cluster by setting an IP address for it.  The way to do this varies according to how containers are attached to Weave Net.

## Manually specifying an IP address when using CNI

The software making a call to CNI has to specify an allocator that allows static IPs.
For instance the [static allocator](https://github.com/containernetworking/plugins/tree/master/plugins/ipam/static)

(At the time of writing there is no straightforward way to ask Kubernetes to do this.)

## Manually specifying an IP address when using the Weave Net Docker Plugin

Docker has a flag `--ip` to specify an address, e.g.:

    $ docker run --network=mynetwork --ip=172.18.0.22 ...

## Manually specifying an IP address when using the Weave Net Docker Proxy

You can specify an IP address and a network explicitly, using Classless Inter-Domain Routing or [CIDR notation](https://en.wikipedia.org/wiki/Classless_Inter-Domain_Routing).

For example, we can launch a couple of containers on `$HOST1` and
`$HOST2`, respectively, with specified IP addresses, as follows...

On `$HOST1`:

    host1$ docker run -e WEAVE_CIDR=10.2.1.1/24 -ti alpine:latest
    root@7ca0f6ecf59f:/#

And `$HOST2`:

    host2$ docker run -e WEAVE_CIDR=10.2.1.2/24 -ti alpine:latest
    root@04c4831fafd3:/#

Then test that the container on `$HOST2` can be reached from the container on `$HOST1`:

    root@7ca0f6ecf59f:/# ping -c 1 -q 10.2.1.2
    PING 10.2.1.2 (10.2.1.2): 48 data bytes
    --- 10.2.1.2 ping statistics ---
    1 packets transmitted, 1 packets received, 0% packet loss
    round-trip min/avg/max/stddev = 1.048/1.048/1.048/0.000 ms

And in the other direction...

    root@04c4831fafd3:/# ping -c 1 -q 10.2.1.1
    PING 10.2.1.1 (10.2.1.1): 48 data bytes
    --- 10.2.1.1 ping statistics ---
    1 packets transmitted, 1 packets received, 0% packet loss
    round-trip min/avg/max/stddev = 1.034/1.034/1.034/0.000 ms

The IP addresses and netmasks can be set to anything, but ensure they don’t conflict with any of the IP ranges in use on the hosts or with IP addresses used by any external services to which the hosts or containers may need to connect. 

Individual IP addresses given to containers must, of course, be unique. If you pick an address that the automatic allocator has already assigned a warning appears.

**See Also**

 * [Managing Services - Exporting, Importing, Binding and Routing]({{ '/tasks/manage/service-management' | relative_url }})
 * [Allocating IPs in a Specific Range]({{ '/tasks/ipam/configuring-weave' | relative_url }}) 
 * [Automatic IP Address Management]({{ '/tasks/ipam/ipam' | relative_url }})   

