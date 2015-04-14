# IP Address Management

## How does it work?

This document builds upon
https://github.com/zettio/weave/wiki/IP-allocation-design.

### Data Structures

The top-level Allocator holds two main data structures:

#### Ring

This is the CRDT holding all ownership of address ranges to peers via
tokens.


#### Space

Package `space` holds detailed information on address ranges owned by this peer.

Operations on addresses, such as deciding whether one address is
within a space, are done by converting the 4-byte IP address into a
4-byte unsigned integer and then doing ordinary arithmetic on that
integer.

The operation to Split() one space into two, to give space to another
Peer, is conceptually simple but the implementation is fiddly to
maintain the various lists and limits within MutableSpace. Perhaps a
different free-list implementation would make this easier.



#### Allocator

We need to be able to release any allocations when a container dies, so
Allocator retains a list of those, in a map `owned` indexed by container ID.

When we run out of free addresses we ask another peer to donate space
and wait for it to get back to us, so we have a list of outstanding
'get' requests.  There is also a list recording outstanding claims of
specific addresses; currently this is only needed until we hear of
some ownership on the ring.

### Retries

There is no specific logic to time-out requests such as "give me some
space": the logic will be re-run and the request re-sent next time
something else happens (e.g. it receives another request, or some
periodic gossip). This means we may send out requests more frequently
than required, but this is innoccuous and it keeps things simple.


## External command interface

IPAM exposes an interface so programs such as the `weave` script can
call upon it to perform functions. This interface operates over the
http protcol on port 6784 (same as used for `weave status`).

The operations supported by this interface are:

  * GET /ip/<containerid> - return a CIDR-format address for the
    container with ID <containerid>.  This ID should be the full
    long-format hex number ID that Docker has given it.  If you call
    this multiple times for the same container it will always return
    the same address. The return value is in CIDR format (preparatory
    for future extension to support multiple subnets). Does not return
    until an address is available (or the allocator shuts down)
  * PUT /ip/<containerid>/<ip> - state that address <ip> is associated
    with <containerid>.  If you send an address outside of the space
    managed by IPAM then this request is ignored.
  * DELETE /ip/<containerid>/<ip> - state that address <ip> is no
    longer associated with <containerid>
  * DELETE /ip/<containerid>/* - equivalent to calling DELETE for all
    ip addresses associated with <containerid>

## Elections

At start-up, nobody owns any of the address space.  An election is
triggered by some peer being asked to allocate or claim an address.

If a peer elects itself as leader, then it can respond to the request directly.

However, if the election is won by some different peer, then the peer
that has the request must wait until the leader takes control before
it can request space.

The peer that triggered the election sends a message to the peer it
has elected.  That peer then re-runs the election, to avoid races
where further peers have joined the group and come to a different conclusion.


## Concurrency

Everything is hung off Allocator, which runs as a single-threaded
Actor, so no locks are used around data structures.

## Other open questions

Currently there is no fall-back range if you don't specify one; it
would be better to pick from a set of possibilities (checking
the range is currently unused before we start using it).

How to use IPAM for WeaveDNS?  It needs its own special subnet.

How should we add support for subnets in general?

We get a bit of noise in the weave logs from containers going down,
now that the weave script is calling ethtool and curl via containers.

If we hear that a container has died we should knock it out of pending?

Interrogate Docker to check container exists and is alive at the point we assign an IP

[It would be good to move PeerName out of package router into a more
central package so both ipam and router can depend on it.]
