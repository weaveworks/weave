# IP Address Management

## How does it work?

This document builds upon
https://github.com/zettio/weave/wiki/IP-allocation-design, which
should be read first to understand the terms and concepts used.

### Data Structures

There are three levels in the CRDT data structure:

Allocator - controls my own allocations and has a view of everyone else's
PeerSpace - all reservations for one Peer
Space     - one contiguous slice of the address space

PeerSpace and Space are interfaces which each have two implementations
- one representing 'our' info which can change as addresses are
allocated and released, and one representing someone else's data which
we only update via the CRDT mechanism.  The concrete classes are
MinSpace/MutableSpace and PeerSpaceSet/OurSpaceSet, respectively.

In addition to the CRDT, Allocator holds information on its own name
and UID, a record of a past life (if observed), a map of which
container owns which IP, and lists of outstanding claims, allocations
and requests.

#### Space implementations

MinSpace is also used to ship data around over the gossip mechanism.

MutableSpace is used for allocation, so it has a free list, currently
a simple slice of IP addresses.
[further scope for enhancement: replace the free list with a bit-mask,
or a run-length-encoded list of free/occupied runs]

Operations on addresses, such as deciding whether one address is
within a space, or whether one space is 'heir' to another, are done by
converting the 4-byte IP address into a 4-byte unsigned integer and
then doing ordinary arithmetic on that integer.

The operation to Split() one space into two, to give space to another
Peer, is conceptually simple but the implementation is fiddly to
maintain the various lists and limits within MutableSpace. Perhaps a
different free-list implementation would make this easier.

#### SpaceSet implementations

PeerSpaceSet and MutableSpaceSet share the same data fields, but the
'spaces' slice will have MinSpace elements for PeerSpaceSet and
MutableSpace elements for MutableSpaceSet.

Each SpaceSet holds its PeerName and UID which, together, uniquely
identify the process that sent it.  As Peers restart we will see the
same PeerName appear with a different UID.

PeerSpaceSet has a lastSeen time which we use to time-out peers we
haven't heard about in a long time. However, we trigger first off the
underlying topology gossip which tells us when nobody is connected to
a peer and set a 'maybeDead' flag.  Only if a Peer is 'maybeDead' and
timed out do we erect a 'tombstone' so that someone will reclaim the
space.
[Is this over-complicated?  Can't remember why we do the maybeDead thing]

MutableSpaceSet increments its version number on every change. [Should
we do extra work to reduce the number of increments?  When does the
field wrap round?  Wrap-around is fatal to the CRDT algorithm.]

[Currently, MutableSpaceSet.GiveUpSpace() does not follow the pattern
suggested in the design doc of favouring smaller spaces]

#### Allocator

We need to be able to release any allocations when a container dies, so
Allocator retains a list of those, in a map indexed by container ID.

When Allocator first detects a leak, i.e. a space with no owner, it
puts it in its 'leaked' map against a timestamp.  In this way it can
wait to see if anyone claims it and, if none, see if this Allocator
should claim it.  It is necessary to repeat this check on the same
leaked space, because we may inherit a space that makes us the heir to
another space that was leaked a while ago.  Meanwhile, any spaces
which are claimed by another peer are removed from the 'leaked' map.

Allocator.considerOurPosition() performs all of these repeated steps,
and is called on a timer.

Another piece of housekeeping we have to do is to look at individual
IP addresses which are claimed (usually by pre-existing containers on
this host). Again, we may not be able to assert ownership immediately
so we keep a 'claims' slice to examine periodically.

Allocator maintains a list of 'inflight' requests so it can avoid
sending the same request twice, and to match up donations against
requests. Donations do not state why they were sent.

Allocator broadcasts its own entry in the CRDT any time its has-free
state or set of reservations changes.

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
    for future extension to support multiple subnets). If all
    addresses are in use, returns http status 503
  * PUT /ip/<containerid>/<ip> - state that address <ip> is associated
    with <containerid>.  If you send an address outside of the space
    managed by IPAM then this request is ignored.
  * DELETE /ip/<containerid>/<ip> - state that address <ip> is no
    longer associated with <containerid>
  * DELETE /ip/<containerid>/* - equivalent to calling DELETE for all
    ip addresses associated with <containerid>

[what happens if you call PUT several times on the same container with
different addreses? what should GET subsequently return?]

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

The peer that sends the message sets a timeout, and if it does not
hear back that someone has allocated some space, then it will re-run
the election.

## Concurrency

Everything is hung off Allocator, which runs as a single-threaded
Actor, so no locks are used around data structures.

## Other open questions

What should the flag to control the allocation "universe" be?
Currently it is `--alloc`.

Currently there is a fall-back range if you don't specify one; it
would be better to pick from a set of possibilities so we can check
the range is currently unused before we start using it.  Docker does
this.

How to use IPAM for WeaveDNS?  It needs its own special subnet.

How should we add support for subnets in general?

We get a lot of noise in the weave logs from containers going down,
now that the weave script is calling ethtool and curl via containers.

When you claim an address, free addresses in the gap are subsequently
allocated in descending order.

[It would be good to move PeerName out of package router into a more
central package so both ipam and router can depend on it.]

[Perhaps Allocators should include the overal Universe in their gossip
so they can detech mismatches.]

[Someone making a request should be able to cancel it]

[There are a lot of `errors.New()` calls in the code; should probably
re-write them to use specific error types]

[Could randomise the process for picking a peer to talk to]
