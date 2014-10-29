---
title: How Weave works
layout: default
---

## How does it work?

Weave creates a network bridge on the host. Each container is
connected to that bridge via a veth pair, the container side of which
is given the IP address & netmask supplied in 'weave run'. Also
connected to the bridge is the weave router container.

A weave router captures Ethernet packets from its bridge-connected
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
peer. The topology information captures which peers are connected to
which other peers; weave can route packets in partially connected
networks with changing topology.

Weave routers establish TCP connections to each other, over which they
perform a protocol handshake and subsequently exchange topology
information. These connections are encrypted if so configured. Peers
also establish UDP "connections", possibly encrypted, for the
aforementioned packet forwarding. These "connections" are duplex and
can traverse firewalls.

More details on the inner workings of weave can be found in the
[architecture documentation](https://github.com/zettio/weave/blob/master/docs/architecture.txt).
