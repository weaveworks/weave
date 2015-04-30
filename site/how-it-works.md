---
title: How Weave works
layout: default
---

## How does it work?

A weave network consists of a number of 'peers' - weave routers
residing on different hosts. Each peer has a name, which tends to
remain the same over restarts, a human friendly nickname for use in
status and logging output and a unique identifier (UID) which is
different each time it is run.  These are opaque identifiers as far as
the router is concerned, although the name defaults to a MAC address.

Weave routers establish TCP connections to each other, over which they
perform a protocol handshake and subsequently exchange
[topology](#topology) information. These connections are encrypted if
so configured. Peers also establish UDP "connections", possibly
encrypted, which carry encapsulated network packets. These
"connections" are duplex and can traverse firewalls.

Weave creates a network bridge on the host. Each container is
connected to that bridge via a veth pair, the container side of which
is given the IP address & netmask supplied in 'weave run'. Also
connected to the bridge is the weave router container.

A weave router captures Ethernet packets from its bridge-connected
interface in promiscuous mode, using 'pcap'. This typically excludes
traffic between local containers, and between the host and local
containers, all of which is routed straight over the bridge by the
kernel. Captured packets are forwarded over UDP to weave router peers
running on other hosts. On receipt of such a packet, a router injects
the packet on its bridge interface using 'pcap' and/or forwards the
packet to peers.

Weave routers learn which peer host a particular MAC address resides
on. They combine this knowledge with topology information in order to
make routing decisions and thus avoid forwarding every packet to every
peer. Weave can route packets in partially connected networks with
changing topology. For example, in this network, peer 1 is connected
directly to 2 and 3, but if 1 needs to send a packet to 4 or 5 it must
first send it to peer 3:
![Partially connected Weave Network](images/top-diag1.png "Partially connected Weave Network")

### <a name="encapsulation"></a>Encapsulation

When the weave router forwards packets, the encapsulation looks
something like this:

    +-----------------------------------+
    | Name of sending peer              |
    +-----------------------------------+
    | Frame 1: Name of capturing peer   |
    +-----------------------------------+
    | Frame 1: Name of destination peer |
    +-----------------------------------+
    | Frame 1: Captured payload length  |
    +-----------------------------------+
    | Frame 1: Captured payload         |
    +-----------------------------------+
    | Frame 2: Name of capturing peer   |
    +-----------------------------------+
    | Frame 2: Name of destination peer |
    +-----------------------------------+
    | Frame 2: Captured payload length  |
    +-----------------------------------+
    | Frame 2: Captured payload         |
    +-----------------------------------+
    |                ...                |
    +-----------------------------------+
    | Frame N: Name of capturing peer   |
    +-----------------------------------+
    | Frame N: Name of destination peer |
    +-----------------------------------+
    | Frame N: Captured payload length  |
    +-----------------------------------+
    | Frame N: Captured payload         |
    +-----------------------------------+

The name of the sending peer enables the receiving peer to identify
who sent this UDP packet. This is followed by the meta data and
payload for one or more captured frames. The router performs batching:
if it captures several frames very quickly that all need forwarding to
the same peer, it fits as many of them as possible into a single
UDP packet.

The meta data for each frame contains the names of the capturing and
destination peers. Since the name of the capturing peer name is
associated with the source MAC of the captured payload, it allows
receiving peers to build up their mappings of which client MAC
addresses are local to which peers. The destination peer name enables
the receiving peer to identify whether this frame is destined for
itself or whether it should be forwarded on to some other peer,
accommodating multi-hop routing. This works even when the receiving
intermediate peer has no knowledge of the destination MAC: only the
original capturing peer needs to determine the destination peer from
the MAC. This way weave peers never need to exchange the MAC addresses
of clients and need not take any special action for ARP traffic and
MAC discovery.

### <a name="topology"></a>Topology

The topology information captures which peers are connected to which
other peers. Weave peers communicate their knowledge of the topology
(and changes to it) to others, so that all peers learn about the
entire topology. This communication occurs over the TCP links between
peers, using a) spanning-tree based broadcast mechanism, and b) a
neighour gossip mechanism.

Topology messages are sent by a peer...

- when a connection has been added; if the remote peer appears to be
  new to the network, the entire topology is sent to it, and an
  incremental update, containing information on just the two peers at
  the ends of the connection, is broadcast,
- when a connection has been marked as 'established', indicating that
  the remote peer can receive UDP traffic from the peer; an update
  containing just information about the local peer is broadcast,
- when a connection has been torn down; an update containing just
  information about the local peer is broadcast,
- periodically, on a timer, the entire topology is "gossiped" to a
  subset of neighbours, based on a topology-sensitive random
  distribution. This is done in case some of the aforementioned
  broadcasts do not reach all peers, due to rapid changes in the
  topology causing broadcast routing tables to become outdated.

The receiver of a topology update merges that update with its own
topology model, adding peers hitherto unknown to it, and updating
peers for which the update contains a more recent version than known
to it. If there were any such new/updated peers, and the topology
update was received over gossip (rather than broadcast), then an
improved update containing them is gossiped.

If the update mentions a peer that the receiver does not know, then
the entire update is ignored.

#### Message details
Every gossip message is structured as follows:

    +-----------------------------------+
    | 1-byte message type - Gossip      |
    +-----------------------------------+
    | 4-byte Gossip channel - Topology  |
    +-----------------------------------+
    | Peer Name of source               |
    +-----------------------------------+
    | Gossip payload (topology update)  |
    +-----------------------------------+

The topology update payload is laid out like this:

    +-----------------------------------+
    | Peer 1: Name                      |
    +-----------------------------------+
    | Peer 1: NickName                  |
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
    | Peer N: NickName                  |
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
    | Connection 1: Outbound            |
    +-----------------------------------+
    | Connection 1: Established         |
    +-----------------------------------+
    | Connection 2: Remote Peer Name    |
    +-----------------------------------+
    | Connection 2: Remote IP address   |
    +-----------------------------------+
    | Connection 2: Outbound            |
    +-----------------------------------+
    | Connection 2: Established         |
    +-----------------------------------+
    |                ...                |
    +-----------------------------------+
    | Connection N: Remote Peer Name    |
    +-----------------------------------+
    | Connection N: Remote IP address   |
    +-----------------------------------+
    | Connection N: Outbound            |
    +-----------------------------------+
    | Connection N: Established         |
    +-----------------------------------+

#### Removal of peers
If a peer, after receiving a topology update, sees that another peer
no longer has any connections within the network, it drops all
knowledge of that second peer.

#### Out-of-date topology
The propagation of topology changes to all peers is not instantaneous,
so it is very possible for a node elsewhere in the network to have an
out-of-date view.

If the destination peer for a packet is still reachable, then
out-of-date topology can result in it taking a less efficient route.

If the out-of-date topology makes it look as if the destination peer
is not reachable, then the packet is dropped.  For most protocols
(e.g. TCP), the transmission will be retried a short time later, by
which time the topology should have updated.

### <a name="crypto"></a>Crypto

Weave can be configured to encrypt both the data passing over the TCP
connections and the payloads of UDP packets sent between peers. This
is accomplished using the [NaCl](http://nacl.cr.yp.to/) crypto
libraries, employing Curve25519, XSalsa20 and Poly1305 to encrypt and
authenticate messages. Weave protects against injection and replay
attacks for traffic forwarded between peers.

NaCl was selected because of its good reputation both in terms of
selection and implementation of ciphers, but equally importantly, its
clear APIs, good documentation and high-quality
[go implementation](https://godoc.org/golang.org/x/crypto/nacl). It is
quite difficult to use NaCl incorrectly. Contrast this with libraries
such as OpenSSL where the library and its APIs are vast in size,
poorly documented, and easily used wrongly.

There are some similarities between Weave's crypto and
[TLS](https://tools.ietf.org/html/rfc4346). We do not need to cater
for multiple cipher suites, certificate exchange and other
requirements emanating from X509, and a number of other features. This
simplifies the protocol and implementation considerably. On the other
hand, we need to support UDP transports, and while there are
extensions to TLS such as [DTLS](https://tools.ietf.org/html/rfc4347)
which can operate over UDP, these are not widely implemented and
deployed.

#### Establishing the Ephemeral Session Key

For every connection between peers, a fresh public/private key pair is
created at both ends, using NaCl's `GenerateKey` function. The public
key portion is sent to the other end as part of the initial handshake
performed over TCP. Peers that were started with a password do not
continue with connection establishment unless they receive a public
key from the remote peer. Thus either all peers in a weave network
must be supplied with a password, or none.

When a peer has received a public key from the remote peer, it uses
this to form the ephemeral session key for this connection. The public
key from the remote peer is combined with the private key for the
local peer in the usual Diffie-Hellman way, resulting in both peers
arriving at the same shared key. To this is appended the supplied
password, and the result is hashed through SHA256, to form the final
ephemeral session key. Thus the supplied password is never exchanged
directly, and is thoroughly mixed into the shared secret. The shared
key formed by Diffie-Hellman is 256 bits long, appending the password
to this obviously makes it longer by an unknown amount, and the use of
SHA256 reduces this back to 256 bits, to form the final ephemeral
session key. This late combination with the password eliminates "Man
In The Middle" attacks: sniffing the public key exchange between the
two peers and faking their responses will not grant an attacker
knowledge of the password, and so an attacker would not be able to
form valid ephemeral session keys.

The same ephemeral session key is used for both TCP and UDP traffic
between two peers.

#### TCP

TCP connection are only used to exchange topology information between
peers, via a message-based protocol. Encryption of each message is
carried out by NaCl's `secretbox.Seal` function using the ephemeral
session key and a nonce. The nonce contains the message sequence
number, which is incremented for every message sent, and a bit
indicating the polarity of the connection at the sender ('1' for
outbound). The latter is required by the
[NaCl Security Model](http://nacl.cr.yp.to/box.html) in order to
ensure that the two ends of the connection do not use the same nonces.

Decryption of a message at the receiver is carried out by NaCl's
`secretbox.Open` function using the ephemeral session key and a
nonce. The receiver maintains its own message sequence number, which
it increments for every message it decrypted successfully. The nonce
is constructed from that sequence number and the connection
polarity. As a result the receiver will only be able to decrypt a
message if it has the expected sequence number. This prevents replay
attacks.

#### UDP

UDP connections carry captured traffic between peers. For a UDP packet
sent between peers that are using crypto, the encapsulation looks as
follows:

    +-----------------------------------+
    | Name of sending peer              |
    +-----------------------------------+
    | Message Sequence No and flags     |
    +-----------------------------------+
    | NaCl SecretBox overheads          |
    +-----------------------------------+ -+
    | Frame 1: Name of capturing peer   |  |
    +-----------------------------------+  | This section is encrypted
    | Frame 1: Name of destination peer |  | using the ephemeral session
    +-----------------------------------+  | key between the weave peers
    | Frame 1: Captured payload length  |  | sending and receiving this
    +-----------------------------------+  | packet.
    | Frame 1: Captured payload         |  |
    +-----------------------------------+  |
    | Frame 2: Name of capturing peer   |  |
    +-----------------------------------+  |
    | Frame 2: Name of destination peer |  |
    +-----------------------------------+  |
    | Frame 2: Captured payload length  |  |
    +-----------------------------------+  |
    | Frame 2: Captured payload         |  |
    +-----------------------------------+  |
    |                ...                |  |
    +-----------------------------------+  |
    | Frame N: Name of capturing peer   |  |
    +-----------------------------------+  |
    | Frame N: Name of destination peer |  |
    +-----------------------------------+  |
    | Frame N: Captured payload length  |  |
    +-----------------------------------+  |
    | Frame N: Captured payload         |  |
    +-----------------------------------+ -+

This is very similar to the [non-crypto encapsulation](#encapsulation).

All of the frames on a connection are encrypted with the same
ephemeral session key, and a nonce constructed from a message sequence
number, flags and the connection polarity. This is very similar to the
TCP encryption scheme, and encryption is again done with the NaCl
`secretbox.Seal` function. The main difference is that the message
sequence number and flags are transmitted as part of the message,
unencrypted.

The receiver uses the name of the sending peer to determine which
ephemeral session key and local cryptographic state to use for
decryption. Frames which are to be forwarded on to some further peer
will be re-encrypted with the relevant ephemeral session keys for the
onward connections. Thus all traffic is fully decrypted on every peer
it passes through.

Decryption is once again carried out by NaCl's `secretbox.Open`
function using the ephemeral session key and nonce. The latter is
constructed from the message sequence number and flags that appeared
in the unencrypted portion of the received message, and the connection
polarity.

To guard against replay attacks, the receiver maintains some state in
which it remembers the highest message sequence number seen. It could
simply reject messages with lower sequence numbers, but that could
result in excessive message loss when messages are re-ordered. The
receiver therefore additionally maintains a set of received message
sequence numbers in a window below the highest number seen, and only
rejects messages with a sequence number below that window, or
contained in the set. The window spans at least 2^20 message sequence
numbers, and hence any re-ordering between the most recent ~1 million
messages is handled without dropping messages.

### Further reading
More details on the inner workings of weave can be found in the
[architecture documentation](https://github.com/weaveworks/weave/blob/master/docs/architecture.txt).
