---
title: Integrating a Host Network with Weave
layout: default
---

Weave application networks can be integrated with an external host network, establishing connectivity between the host and with application containers running anywhere.

For example, returning to the [netcat example]( /site/using-weave/intro-example.md), you’ve now decided that you need to have the application containers that are running on `$HOST2` accessible by other hosts and containers. 

On `$HOST2` run:

    host2$ weave expose
    10.2.1.132

This command grants the host access to all of the application containers in the default subnet. An IP address is allocated by Weave especially for that purpose, and is returned after running `weave expose`. 

Now you are able to ping the host:

    host2$ ping 10.2.1.132

And you can also ping the `a1` netcat application container residing on `$HOST1`:

    host2$ ping $(weave dns-lookup a1)


###Exposing Multiple Subnets

Multiple subnet addresses can be exposed or hidden with a single command:

    host2$ weave expose net:default net:10.2.2.0/24
    10.2.1.132 10.2.2.130
    host2$ weave hide   net:default net:10.2.2.0/24
    10.2.1.132 10.2.2.130


###Adding Exposed Addresses to weavedns

Exposed addresses can also be added to `weavedns` by supplying fully qualified domain names:

    host2$ weave expose -h exposed.weave.local
    10.2.1.132


**See Also**

 * [Deploying Applications To Weave Net](/site/using-weave/deploying-applications.md)
 * [Managing Services in Weave: Exporting, Importing, Binding and Routing](/site/using-weave/service-management.md)