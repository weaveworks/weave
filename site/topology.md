---
title: Topology
layout: default
---

## Topology

When we talk about the topology of a Weave network, we mean how peers
are connected to other peers.  Each peer uses this information to make
routing decisions for packets that have to flow from point to point
across the network.

For example, in this network, peer 1 is connected directly to 2 and 3,
but if 1 needs to send a packet to 4 or 5 it must first send it to
peer 3:
![diagram showing five peers in a weave network][diagram1]
Weave peers communicate their own topology with neighbours, who then
pass on changes to their neighbours, and so on, until the entire
network knows about any change.  

### Communication
Topology is communicated over the TCP links between peers.
There are two message types:

#### FetchAll 
This carries no payload. The receiver responds with the
entire topology model as the receiver has it.  A new peer joining the
network sends this message after it has completed initial handshakes.

#### Update
This carries a topology payload. This is sent:
  * upon receipt of a FetchAll message, as above, 
  * when a connection is added - the update will contain the two peers
that have just connected
  * when a connection is deleted - the update will contain just the peer
that lost the connection

The 
receiver merges it with its own topology model. If the payload is a
subset of the receiver's topology, no further action is
taken. Otherwise, the receiver sends out to all its connections an
"improved" update:

 - elements which the original payload added to the
   receiver are included
 - elements which the original payload updated in the
   receiver are included
 - elements which are equal between the receiver and
   the payload are not included
 - elements where the payload was older than the
   receiver's version are updated

If the update mentions a peer that the receiver has never heard of,
then the entire update is rejected and the receiver will send a
FetchAll message back to the sender.

### Message details
A topology update message is laid out like this:

    +-----------------------------------+
    | 1-byte message type               |
    +-----------------------------------+
    | Peer 1: Name                      |
    +-----------------------------------+
    | Peer 1: UID                       |
    +-----------------------------------+
    | Peer 1: Version number            |
    +-----------------------------------+
    | Peer 1: List of connections       |
    +-----------------------------------+
    |                ...                |
    +-----------------------------------+
    | Peer N: Name                      |
    +-----------------------------------+
    | Peer N: UID                       |
    +-----------------------------------+
    | Peer N: Version number            |
    +-----------------------------------+
    | Peer N: List of connections       |
    +-----------------------------------+

Each List of connections is encapsulated as a byte buffer, within
which the structure is:

    +-----------------------------------+
    | Connection 1: Remote Peer Name    |
    +-----------------------------------+
    | Connection 1: Remote IP address   |
    +-----------------------------------+
    | Connection 2: Remote Peer Name    |
    +-----------------------------------+
    | Connection 2: Remote IP address   |
    +-----------------------------------+
    |                ...                |
    +-----------------------------------+
    | Connection N: Remote Peer Name    |
    +-----------------------------------+
    | Connection N: Remote IP address   |
    +-----------------------------------+

### Removal of peers
If a peer, after receiving a topology update, sees that another peer
no longer has any connections within the network, it will drop all
knowledge of that second peer.


### Out-of-date topology
The peer-to-peer passing of updates is not instantaneous, so it is
very posisble for a node elsewhere in the network to have an
out-of-date view.

If the destination peer for a packet is still reachable, then
out-of-date topology can result in it taking a less efficient route.

If the out-of-date topology makes it look as if the destination peer
is not reachable, then the packet will be dropped.  For most protocols
(e.g. TCP), the transmission will be retried a short time later, by
which time the topology should have updated.

[diagram1]: images/top-diag1.png "Diagram 1"
