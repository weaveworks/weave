# IP Address Management

## How does it work?

See also https://github.com/zettio/weave/wiki/IP-allocation-design

In this document I use the word 'allocation' to refer to a specific
address being assigned so it can be given to a container, and
'reservation' to refer to a block of addresses being given to a Peer
to subsequently allocate.

### Data Structures

There are three levels in the data structure:

Allocator - controls my own allocations and has a view of everyone else's
PeerSpace - all reservations for one Peer
Space     - one contiguous slice of the address space

PeerSpace and Space are interfaces which each have two implementations
- one representing 'our' info which can change as addresses are
allocated and released, and one representing someone else's data which
we only update via the CRDT mechanism.  The concrete classes are
MinSpace/MutableSpace and PeerSpaceSet/OurSpaceSet, respectively.

#### Space implementations

MinSpace is also used to ship data around over the gossip mechanism.

MutableSpace is used for allocation, so it has a free list.  We also
need to be able to release any allocations when a container dies, so
it retains a list of those. [this list could equally be maintained at
Allocator level, but at present it participates in the Split()
operation] 
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

[MutableSpace has a RWMutex which may not be necessary - probably all
accesses are via the owning OurSpaceSet which has its own lock]

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
suggested in the design doc of favouring smaller spaces, nor does it
give up its last free address.]

#### Allocator

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

## Concurrency

Everything is hung off Allocator, which locks around every call from
outside, and also its own periodic timer routine.  Concurrent calls
can come in from the Gossip system (over different connections at the
same time) and the http interface.

All data structures (except MinSpace) are protected by
RWMutex. MinSpace is not locked because it is read-only except when
manipulated by MutableSpaceSet which has its own lock.

[do we need to lock MutableSpaceSet?  There is only one, owned by
Allocator, which is already locking]

Go's locks are not re-entrant, so we have some rules to avoid
deadlock: exposed functions (start with uppercase) take a lock;
internal functions never take a lock and never call an exposed
function.

## Other open questions

What should the flag to control the allocation "universe" be?
Currently it is `--alloc`.

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
