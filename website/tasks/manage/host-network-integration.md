---
title: Integrating with the Host Network
menu_order: 20
search_type: Documentation
---

Weave application networks can be integrated with an external host network, establishing connectivity between the host and with application containers running anywhere.

For example, returning to the [netcat example]({{ '/install/using-weave' | relative_url }}), you’ve now decided that you need to have the application containers that are running on `$HOST2` accessible by other hosts and containers. 

On `$HOST2` run:

    host2$ weave expose
    10.2.1.132

This command grants the host access to all of the application containers in the default subnet. An IP address is allocated by Weave Net especially for that purpose, and is returned after running `weave expose`. 

Now you are able to ping the host:

    host2$ ping 10.2.1.132

And you can also ping the `a1` netcat application container residing on `$HOST1`:

    host2$ ping $(weave dns-lookup a1)

### Exposing Multiple Subnets

Multiple subnet addresses can be exposed or hidden using a single command:

    host2$ weave expose net:default net:10.2.2.0/24
    10.2.1.132 10.2.2.130
    host2$ weave hide   net:default net:10.2.2.0/24
    10.2.1.132 10.2.2.130

### Adding Exposed Addresses to weaveDNS

Exposed addresses can also be added to weaveDNS by supplying fully qualified domain names:

    host2$ weave expose -h exposed.weave.local
    10.2.1.132

### <a name="routing"></a>Routing from Another Host

After running `weave expose`, you can use Linux routing to provide
access to the Weave network from hosts that are not running Weave Net:

    ip route add <network-cidr> via <exposing-host>

Where,

 * `<network-cidr>` is an IP address range in use on Weave Net,
for example,  `10.2.0.0/16` or `10.32.0.0/12` and,
 * `<exposing-host>` is the address of the machine on which you ran `weave expose`.

>**Note:** You must ensure that the [IP subnet used by Weave
Net]({{ '/tasks/ipam/ipam#range' | relative_url }}) does not clash with anything on those other
hosts.


**See Also**

 * [Launching Weave Net]({{ '/install/using-weave' | relative_url }})
 * [General information on IP Addresses, Routes and Networks]({{ '/concepts/ip-addresses' | relative_url }})
 * [Managing Services - Exporting, Importing, Binding and Routing]({{ '/tasks/manage/service-management' | relative_url }})
