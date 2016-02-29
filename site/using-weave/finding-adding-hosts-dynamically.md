---
title: Adding and Removing Hosts Dynamically
layout: default
---


To add a host to an existing Weave network, simply launch 
Weave on the host, and then supply the address of at least 
one host. Weave automatically discovers any other hosts in 
the network and  establishes connections with them if it 
can (in order to avoid unnecessary multi-hop routing).

In some situations all existing Weave hosts may be 
unreachable from the new host due to firewalls, etc. 
However, it is still possible to add the new host, 
provided that inverse connections, for example, 
from existing hosts to the new hosts, are available. 

To accomplish this, launch Weave onto the new host 
without supplying any additional addresses.  And then, from one 
of the existing hosts run:

    host# weave connect $NEW_HOST

Other hosts in the Weave network will automatically attempt
to establish connections to the new host as well. 

Alternatively, you can also instruct a peer to forget a 
particular host specified to it via `weave launch` or 
`weave connect` by running:

    host# weave forget $DECOMMISSIONED_HOST

This prevents the peer from trying to reconnect to that host 
once connectivity to it is lost, and therfore can be used 
to administratively remove any decommissioned peers 
from the network.

Hosts may also be bulk-replaced. All existing hosts 
will be forgotten, and the new hosts will be added:

    host# weave connect --replace $NEW_HOST1 $NEW_HOST2

For complete control over the peer topology, automatic 
discovery can be disabled using the `--no-discovery` 
option with `weave launch`. 

If discovery if disabled, Weave only connects to the 
addresses specified at launch time and with `weave connect`.

A list of all hosts that a peer has been asked to connect 
to with `weave launch` and `weave connect` 
can be obtained with

    host# weave status targets

**See Also** 

 * [Enabling Multi-Cloud, Multi-Hop Networking and Routing](/site/using-weave/multi-cloud-multi-hop.md)
 * [Stopping and Removing Peers](/site/ipam/stop-remove-peers-ipam.md)