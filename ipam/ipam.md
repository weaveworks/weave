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

MinSpace is also used to ship data around over the gossip mechanism.

MutableSpace is used for allocation, so it has a free list.  We also
need to be able to release any allocations when a container dies, so
it retains a list of those. [this list could equally be maintained at
Allocator level, but at present it participates in the Split()
operation] 
[further scope for enhancement: replace the free list with a bit-mask,
or a run-length-encoded list of free/occupied runs]

