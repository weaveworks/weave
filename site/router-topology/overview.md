---
title: How Weave Net Works
layout: default
---



A Weave network consists of a number of 'peers' - Weave routers
residing on different hosts. Each peer has a name, which tends to
remain the same over restarts, a human friendly nickname for use in
status and logging output and a unique identifier (UID) that is
different each time it is run.  These are opaque identifiers as far as
the router is concerned, although the name defaults to a MAC address.

Weave routers establish TCP connections with each other, over which they
perform a protocol handshake and subsequently exchange
[topology](/site/router-topology/network-topology.md) information. 
These connections are encrypted if
so configured. Peers also establish UDP "connections", possibly
encrypted, which carry encapsulated network packets. These
"connections" are duplex and can traverse firewalls.

Weave creates a network bridge on the host. Each container is
connected to that bridge via a veth pair, the container side of which
is given an IP address and netmask supplied either by the user or
by Weave's IP address allocator. Also connected to the bridge is the
Weave router container.

A Weave router captures Ethernet packets from its bridge-connected
interface in promiscuous mode, using 'pcap'. This typically excludes
traffic between local containers, and between the host and local
containers, all of which is routed straight over the bridge by the
kernel. Captured packets are forwarded over UDP to weave router peers
running on other hosts. On receipt of such a packet, a router injects
the packet on its bridge interface using 'pcap' and/or forwards the
packet to peers.

Weave routers learn which peer host a particular MAC address resides
on. They combine this knowledge with topology information in order to
make routing decisions and thus avoid forwarding every packet to every
peer. Weave can route packets in partially connected networks with
changing topology. For example, in this network, peer 1 is connected
directly to 2 and 3, but if 1 needs to send a packet to 4 or 5 it must
first send it to peer 3:

![Partially connected Weave Network](images/top-diag1.png "Partially connected Weave Network")

**See Also**

 * [Weave Router Encapsulation](/site/router-topology/router-encapsulation.md)
 * [How Weave Inteprets Network Topology](/site/router-topology/network-topology.md)