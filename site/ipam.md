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

You can see which address was allocated with
[`weave ps`](troubleshooting.html#list-attached-containers):

    host1# weave ps $C
    a7aff7249393 7a:51:d1:09:21:78 10.2.3.1/24

The `-iprange` parameter is given in [CIDR
notation](http://en.wikipedia.org/wiki/Classless_Inter-Domain_Routing)
format - in this example "/24" means the first 24 bits of the address
form the network address and the allocator is to allocate container
addresses that all start 10.2.3. The ".0" and ".-1" addresses in a
subnet are not used, as required by [RFC
1122](https://tools.ietf.org/html/rfc1122#page-29).

Once you have given a range of addresses to the IP allocator, you must
not use any addresses in that range for anything else.  If, in our
example, you subsequently did `weave run 10.2.3.x/24`, you run the
risk that that address will be allocated to another container, which
will make network traffic delivery intermittent or non-existent for
the containers that share the same IP address. No diagnostic message
is output by weave if you break this rule.

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

To illustrate, suppose you have three hosts, accessible to each other
as `$HOST1`, `$HOST2` and `$HOST3`. You can start weave on those three
hosts with these three commands:

    host1$ weave launch -iprange 10.3.0.0/16 $HOST2 $HOST3

    host2$ weave launch -iprange 10.3.0.0/16 $HOST1 $HOST3

    host3$ weave launch -iprange 10.3.0.0/16 $HOST1 $HOST2

Or, if it is not convenient to name all the other hosts at launch
time, you can give the number of peers like this:

    host1$ weave launch -iprange 10.3.0.0/16 -initpeercount 3

    host2$ weave launch -iprange 10.3.0.0/16 -initpeercount 3 $HOST3

    host3$ weave launch -iprange 10.3.0.0/16 -initpeercount 3 $HOST2

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

Assuming we had started the three peers in the example earlier, and
host3 has caught fire, we can go to one of the other hosts and run:

    host1$ weave rmpeer host3

Weave will take all the IP address ranges owned by host3 and transfer
them to be owned by host1. The name "host3" is resolved via the
'nickname' feature of weave, which defaults to the local host
name. You can see the full set of range assignments in the output from
`weave status`, which will show the internal hex ID and nickname in
brackets, like this:

    Ring [10.2.1.1, 10.2.1.255)
      10.2.1.1 -> 96:e9:e2:2e:2d:bc (host1) (version: 3, free: 126)
      10.2.1.128 -> ea:84:25:9b:31:2e (host2) (version: 2, free: 64)
      10.2.1.192 -> ea:6c:21:09:cf:f0 (host3) (version: 1, free: 62)

So, in our example, we could equivalently do:

    host1$ weave rmpeer ea:6c:21:09:cf:f0

## <a name="troubleshooting"></a>Troubleshooting

The command

    weave status

reports on the current status of the weave router and IP allocator:

````
weave router git-8f675f15c0b5
...
Allocator subnet 10.2.1.0/24
  Free IPs: ~98.0%, 62 local, ~189 remote
Owned Ranges:
  10.2.1.1 -> 96:e9:e2:2e:2d:bc (host1) (v3)
  10.2.1.128 -> ea:84:25:9b:31:2e (host2) (v3)
  10.2.1.192 -> ea:6c:21:09:cf:f0 (host3) (v9)
````

The first section covers the router; see the troubleshooting guide in
the main documentation for full details.

The 'Allocator' section, which is only present if weave has been
started with the `-iprange` option, summarises the overal position and
lists which address ranges have been assigned to which peer. Each
range begins at the address shown and ends just before the next
address, or wraps around at the end of the subnet. The 'v' number
denotes how many times that entry has been updated.

The 'Free IPs' information may be out of date with respect to changes
happening elsewhere in the network.
