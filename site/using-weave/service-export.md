---
title: Exposing Services to the Outside World
layout: default
---

Services running in containers on a Weave network can be made
accessible to the outside world (and, more generally, to other networks)
from any Weave host, irrespective of where the service containers are
located.

Returning to the netcat example service, described in [Deploying Applications]( /site/using-weave/deploying-applications.md), 
you can expose the netcat service running on `HOST1` and make it accessible to the outside world via `$HOST2`. 

First, expose the application network to `$HOST2`, as explained in [Integrating a Host Network with a Weave](/site/using-weave/host-network-integration.md):

    host2$ weave expose
    10.2.1.132

Then add a NAT rule that routes the traffic from the outside world to the destination container service.

    host2$ iptables -t nat -A PREROUTING -p tcp -i eth0 --dport 2211 \
           -j DNAT --to-destination $(weave dns-lookup a1):4422

In this example, it is assumed that the "outside world" is connecting to `$HOST2` via 'eth0'. The TCP traffic to port 2211 on the external IPs will be routed to the 'nc' service running on port 4422 in the container a1.

With the above in place, you can connect to the 'nc' service from anywhere using:

    echo 'Hello, world.' | nc $HOST2 2211

>>**Note:** Due to the way routing is handled in the Linux kernel, this won't work when run *on* `$HOST2`.

Similar NAT rules to the above can be used to expose services not just to the outside world but also to other, internal, networks.



**See Also**

 * [Using Weave Net](/site/using-weave/intro-example.md)
 * [Managing Services in Weave: Exporting, Importing, Binding and Routing](/site/using-weave/service-management.md)