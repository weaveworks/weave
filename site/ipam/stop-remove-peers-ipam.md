---
title: Starting, Stopping and Removing Peers
layout: default
---


You may wish to `weave stop` and re-launch to change some config or to
upgrade to a new version; provided the underlying protocol hasn't
changed it will pick up where it left off and learn from peers in the
network which address ranges it was previously using. If, however, you
run `weave reset` this will remove the peer from the network so
if Weave is run again on that node it will start from scratch.

For failed peers, the `weave rmpeer` command can be used to
permanently remove the ranges allocated to said peer.  This will allow
other peers to allocate IPs in the ranges previously owner by the rm'd
peer, and as such should be used with extreme caution - if the rm'd
peer had transferred some range of IP addresses to another peer but
this is not known to the whole network, or if it later rejoins
the Weave network, the same IP address may be allocated twice.

Assuming we had started the three peers in the example earlier, and
host3 has caught fire, we can go to one of the other hosts and run:

    host1$ weave rmpeer host3

Weave will take all the IP address ranges owned by host3 and transfer
them to be owned by host1. The name "host3" is resolved via the
'nickname' feature of weave, which defaults to the local host
name. Alternatively, one can supply a peer name as shown in `weave
status`.

**See Also**

 * [Address Allocation with IP Address Management (IPAM)](/site/ipam/overview-init-ipam.md)
 * [Automatic Allocation Across Multiple Subnets](/site/ipam/allocation-multi-ipam.md)
 * [Isolating Applications on a Weave Network](/site/using-weave/isolating-applications.md)
 