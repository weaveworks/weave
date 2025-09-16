---
title: Automatic IP Allocation and the Weave Proxy
menu_order: 30
search_type: Documentation
---


If [automatic IP address allocation]({{ '/tasks/ipam/ipam' | relative_url }}) is enabled in Weave Net (by default IPAM is enabled),
then containers started via the proxy are automatically assigned an IP address, *without having to specify any
special environment variables or any other options*.

    host1$ docker run -ti alpine:latest

To use a specific subnet, you can pass a `WEAVE_CIDR` to the container, for example:

    host1$ docker run -ti -e WEAVE_CIDR=net:10.32.2.0/24 alpine:latest

To start a container without connecting it to the Weave network, pass
`WEAVE_CIDR=none`, for example:

    host1$ docker run -ti -e WEAVE_CIDR=none alpine:latest

### Disabling Automatic IP Address Allocation

If you do not want an IP to be assigned by default, the proxy needs to
be passed the `--no-default-ipalloc` flag, for example:

    host1$ weave launch --no-default-ipalloc

In this configuration, containers using no `WEAVE_CIDR` environment
variable are not connected to the Weave network.

Containers started with a `WEAVE_CIDR` environment variable are handled as before.
To automatically assign an address in this mode, start the
container with a blank `WEAVE_CIDR`, for example:

    host1$ docker run -ti -e WEAVE_CIDR="" alpine:latest

**See Also**

 * [Address Allocation with IP Address Management (IPAM)]({{ '/tasks/ipam/ipam' | relative_url }})
 * [How to Manually Specify IP Addresses and Subnets]({{ '/tasks/ipam/manual-ip-address' | relative_url }})
