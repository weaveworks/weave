---
title: Starting, Stopping and Removing Peers
menu_order: 20
---


You may wish to `weave stop` and re-launch to change some config or to
upgrade to a new version. Provided that the underlying protocol hasn't
changed, Weave Net picks up where it left off and learns from peers in
the network which address ranges it was previously using.

If, however, you run `weave reset` this removes the peer from the
network so if Weave Net is run again on that node it will start from
scratch.

For failed peers, the `weave rmpeer` command can be invoked to
permanently remove the ranges allocated to said peers.  This allows
other peers to allocate IPs in the ranges previously owned by the
removed peers, and as such should be used with extreme caution: if the
removed peers had transferred some range of IP addresses to other
peers but this is not known to the whole network, or if some of them
later rejoin the Weave network, the same IP address may be allocated
twice.

Assume you had started the three peers in the
[overview example](/site/ipam.md), and then host3
caught fire, you can go to one of the other hosts and run:

    host1$ weave rmpeer host3
    524288 IPs taken over from host3

Weave Net takes all the IP address ranges owned by host3 and transfers
them to be owned by host1. The name "host3" is resolved via the
'nickname' feature of Weave Net, which defaults to the local host
name. Alternatively, you can supply a peer name as shown in `weave
status`.

**See Also**

 * [Address Allocation with IP Address Management (IPAM)](/site/ipam.md)
 * [Automatic Allocation Across Multiple Subnets](/site/ipam/allocation-multi-ipam.md)
 * [Isolating Applications on a Weave Network](/site/using-weave/application-isolation.md)
