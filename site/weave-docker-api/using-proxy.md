---
title: Using The Weave Docker API Proxy
layout: default
---


When containers are created via the Weave proxy, their entrypoint is 
modified to wait for the Weave network interface to become
available. 

When they are started via the Weave proxy, containers are 
[automatically assigned IP addresses](/site/ipam/overview-init-ipam.md) and connected to the
Weave network.  

###Creating and Starting Containers with the Weave Proxy

To create and start a container via the Weave proxy run:

    host1$ docker run -ti ubuntu

or, equivalently run:

    host1$ docker create -ti ubuntu
    5ef831df61d50a1a49272357155a976595e7268e590f0a2c75693337b14e1382
    host1$ docker start 5ef831df61d50a1a49272357155a976595e7268e590f0a2c75693337b14e1382

Specific IP addresses and networks can be supplied in the `WEAVE_CIDR`
environment variable, for example:

    host1$ docker run -e WEAVE_CIDR=10.2.1.1/24 -ti ubuntu

Multiple IP addresses and networks can be supplied in the `WEAVE_CIDR`
variable by space-separating them, as in
`WEAVE_CIDR="10.2.1.1/24 10.2.2.1/24"`.


###Returning Weave Network Settings Instead of Docker Network Settings

The Docker NetworkSettings (including IP address, MacAddress, and
IPPrefixLen), are still returned when `docker inspect` is run. If you want
`docker inspect` to return the Weave NetworkSettings instead, then the
proxy must be launched using the `--rewrite-inspect` flag. 

This command substitutes the Weave Network settings when the container has a
Weave IP. If a container has more than one Weave IP, then the inspect call
only includes one of them.

    host1$ weave launch-router && weave launch-proxy --rewrite-inspect


###Multicast Traffic and Launching the Weave Proxy

By default, multicast traffic is routed over the Weave network.
To turn this off, e.g. because you want to configure your own multicast
route, add the `--no-multicast-route` flag to `weave launch-proxy`.


**See Also**

 * [Setting Up The Weave Docker API Proxy](/site/weave-docker-api/set-up-proxy.md)
 * [Securing Docker Communications With TLS](securing-proxy.md)
 * [Launching Containers With Weave Run (without the Proxy)](/site/weave-docker-api/launching-without-proxy.md)