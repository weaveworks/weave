---
title: Automatic IP Address Management
layout: default
---

# Automatic IP Address Management

Weave can automatically assign unique IP addresses to each container
across the network. To make this work, weave must be told on startup
what range of addresses to allocate from, for example:

    host1# weave launch -iprange 10.2.3.0/24

The `run`, `start`, `attach`, `expose` and `hide` commands will then
fetch an address automatically if none is specified, i.e.:

    host1# C=$(weave run -ti ubuntu)

Weave will automatically learn when a container has exited
and hence can release its IP address.

Weave shares the IP range across all peers, dynamically according to
their needs.  If a group of peers becomes isolated from the rest (a
partition), they can continue to work with the IP ranges they had
before isolation, and can be re-connected to the rest of the network
and carry on. Note that you must specify the same range with
`-iprange` on each host, and you cannot mix weaves started with and
without -iprange.

### Initialisation

Just once, when you start up the whole network, weave needs a majority
of peers to agree in order to avoid isolated groups starting off
inconsistently. Therefore, you must either supply the list of all
peers in the network to `weave launch` or add the `-initpeercount`
flag to specify how many peers there will be.  It isn't a problem to
over-estimate by a bit, but if you supply a number that is too small
then multiple independent groups may form.

### Stopping and removing peers

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
