---
title: IP Addresses, routes and networks
layout: default
---

# IP Addresses, routes and networks

Weave Net lets you run containers on a private network, and so the IP
addresses those containers use are insulated from the rest of the
Internet, and you don't have to worry about them clashing. Except, if
they actually do clash with some addresses that you'd like the
containers to talk to.

Some definitions:

- _IP_ is the Internet Protocol, the fundamental basis of network
   communication between billions of connected devices.
- The _IP address_ is (for most purposes) the four numbers separated
  by dots, like `192.168.48.12`. Each number is one byte in size, so can
  be between 0 and 255.
- Each IP address lives on a _Network_, which is some set of those
  addresses that all know how talk to each other. The network address
  is some prefix of the IP address, like `192.168.48`. To indicate
  which part of the address indicates the network, we append a slash
  and then the number of bits in the network prefix, like
  `/24`. Another example would be `10.4.2.6/8`, where the network
  prefix is the first 8 bits - `10`. Written out in full, the address
  of this network is `10.0.0.0/8`. The most common prefix lengths are
  8, 16 and 24, but there is nothing stopping you using a /9 network
  or a /26.
- A _route_ is an instruction for how to deal with traffic destined
  for somewhere else - it specifies a Network, and a way to talk to
  that network.  Every device using IP has a table of routes, so for
  any destination address it looks up that table, finds the right
  route, and sends it in the direction indicated.

Quick example: this is the route table for a container that I just started:

````
# ip route show
default via 172.17.42.1 dev eth0 
10.2.2.0/24 dev ethwe  proto kernel  scope link  src 10.2.2.1 
172.17.0.0/16 dev eth0  proto kernel  scope link  src 172.17.0.170 
````

It has two interfaces: one that Docker gave it called `eth0`, and one
that weave gave it called `ethwe`. They are on networks
`172.17.0.0/16` and `10.2.2.0/24` respectively, and if you want to
talk to any other address on those networks then the routing table
tells it to send directly down that interface. If you want to talk to
anything else not matching those rules, the default rule says to send
it to `172.17.42.1` down the eth0 interface.

So, suppose this container wants to talk to another container at
address `10.2.2.9`; it will send down the ethwe interface and weave
Net will take care of routing the traffic. To talk an external server
at address `74.125.133.128`, it looks in its routing table, doesn't
find a match, so uses the default rule.

The default configurations for both weave Net and Docker use [Private
Networks](https://en.wikipedia.org/wiki/Private_network), whose
addresses are never used on the public internet, so that reduces the
chances of overlap. But it could be that you or your hosting provider
are using some of these private addresses in the same range, which would
cause a clash.

Here's an example that came up recently: on `weave launch`, the
following error message appeared:

````
Network 10.32.0.0/12 overlaps with existing route 10.0.0.0/8 on host.
ERROR: Default --ipalloc-range 10.32.0.0/12 overlaps with existing route on host.
You must pick another range and set it on all hosts.
````

As the message says, the default that weave Net would like to use is
`10.32.0.0/12` - a 12-bit prefix so all addresses starting with the bit
pattern 000010100010, or in decimal everything from 10.32 through
10.47. But the user's host already has a route for `10.0.0.0/8`,
which overlaps, because the first 8 bits are the same. If we went
ahead and used the default network, then for an address like
`10.32.5.6` the kernel would never be sure whether this meant the
weave Net network of `10.32.0.0/12` or the hosting network of
10.0.0.0/8.

If you're in this situation, the best option is, as the message says,
to pick a different range. In this example, a different private range
such as `172.30/16` would work.
