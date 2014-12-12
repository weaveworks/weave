---
title: How Weave works
layout: default
---

## How does it work?

A weave network consists of a number of 'peers' - weave routers
residing on different hosts. Each peer has a name, which tends to
remain the same over restarts, and a unique identifier (UID) which is
different each time it is run.  These are opaque identifiers as far as
the router is concerned, although the name defaults to a MAC address.

Weave routers establish TCP connections to each other, over which they
perform a protocol handshake and subsequently exchange topology
information (see below). These connections are encrypted if so
configured. Peers also establish UDP "connections", possibly
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
peer. The topology information captures which peers are connected to
which other peers; weave can route packets in partially connected
networks with changing topology.

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
the same peer, it will fit as many of them as possible into a single
UDP packet.

The meta data for each frame contains the names of the capturing and
destination peers. Since the name of the capturing peer name is
associated with the source MAC of the captured payload, it allows
receivings peers to build up their mappings of which client MAC
addresses are local to which peers. The destination peer name enables
the receiving peer to identify whether this frame is destined for
itself or whether it should be forwarded on to some other peer,
accomodating multi-hop routing. This will work even when the receiving
intermediate peer has no knowledge of the destination MAC: only the
original capturing peer needs to determine the destination peer from
the MAC. This way weave peers never need to exchange the MAC addresses
of clients and need not take any special action for ARP traffic and
MACs discovery.

### Crypto

Weave encrypts traffic over both TCP and UDP between peers. Weave
uses the NaCl crypto libraries, using Curve25519, XSalsa20 and Poly1305
to encrypt and authenticate messages. Weave protects against injection
and replay attacks for traffic forwarded between peers.

NaCl was selected because of its good reputation both in terms of
selection and implementation of ciphers, but equally importantly, its
clear APIs and good documentation. It is quite difficult to use NaCl
incorrectly.  Contrast this with libraries such as OpenSSL where the
size of the library and its APIs are vast, poorly documented, and
easily used wrongly.

#### Establishing the Ephemeral Session Key

When a weave peer has been started with the `-password` option, the
initial handshake between the peers over TCP includes the exchange of
the peers' public keys. Each peer generates a fresh public and private
key pair for each connection, and these are created from the NaCl
crypto library's `GenerateKey` function. Peers that were started with
`-password` do not continue with connection establishment unless they
receive a public key from the remote peer. Thus either all peers in a
weave network must be started with `-password`, or none of them.

When a peer has received a public key from the remote peer, it uses
this to form the ephemeral session key for this connection. The public
key from the remote peer is combined with the private key for the
local peer in the usual Diffie-Hellman way, resulting in both peers
arriving at the same shared key. To this is appended the password from
the command line `-password` option, and the result is hashed through
SHA256, to form the final ephemeral session key. Thus the supplied
password is never exchanged directly, and is thoroughly mixed into the
shared secret. The shared key formed by Diffie-Hellman is 256 bits
long, appending the password to this obviously makes it longer by an
unknown amount, and the use of SHA256 reduces this back to 256 bits,
to form the final ephemeral session key. This late combination with
the password eliminates "Man In The Middle" attacks: sniffing the
public key exchange between the two peers and faking their responses
will not grant an attacker knowledge of the password, and so an
attacker would not be able to form valid ephemeral session keys.

The same ephemeral session key is used for both TCP and UDP traffic
between two peers.

#### TCP

TCP connection are only used to exchange topology information between
peers, via a message-based protocol. The router generates a fresh
192-bit random nonce for every message to be sent, and prepends the
nonce to the encrypted message so that the receiver knows the
nonce. Encryption of each message is carried out using NaCl's
`secretbox.Seal` function using the ephemeral session key. Each TCP
connection has a monotonically incrementing message counter, the
current value of which is included in the encrypted part of the
message. Given the assumption that TCP is reliable and ordered, a
message received via TCP is only acted upon if the message counter in
the received message is the expected message counter. This prevents
replay attacks on the TCP connection.

As TCP connections do not carry captured traffic, minimising message
size or latency is not a major concern, so the potentially substantial
increase in length of messages by prepending the full nonce, or the
cost of generating a fresh random nonce for each message is not
considered likely to cause problems. The random nonces are created by
crypto library's rand module, which implements a cryptographically
secure pseudorandom number generator.

#### UDP

UDP connection carry captured traffic between peers. For a UDP packet
sent between peers that are using crypto, the encapsulation looks as
follows:

    +-----------------------------------+
    | Name of sending peer              |
    +-----------------------------------+
    | Nonce offset and flags            |
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

All of the frames are encrypted with the same ephemeral session key
and all must be decrypted by the receiving peer. Frames which are to
be forwarded on to some further peer will be re-encrypted with the
relevant ephemeral session keys for the onwards connections. Thus all
traffic is fully decrypted on every peer it passes through.
Encryption is again done with the NaCl `secretbox.Seal` function.

The name of the sending peer enables the receiver to identify the peer
who sent this UDP packet, and in turn to determine which ephemeral
session key was used and which nonce, and perform decryption. To avoid
sending a fresh 192-bit nonce with every UDP packet, which would pose
an unacceptable overhead, each UDP packet only carries the lowest 15
bits of the nonce, which is treated as an offset from the "established
nonce". The lifecycle of a nonce for UDP is:

1. A fresh nonce is generated and the most significant 177 bits are
sent to the receiving peer over the TCP connection. These upper most
177 bits are used for the next 2^15 (32768) UDP messages. This is the
"established nonce".

2. Each UDP message carries the lowest 15 bits as a unique counter. Thus
the lowest 15 bits are combined with (appended to) the uppermost 177
bits to form the unique nonce for that message.

3. Once the sending side has sent the 16384'th message on the current
nonce (50% of the way through the available range), it generates a new
nonce (upper 177 bits only) and sends that over the TCP connection,
thus hopefully ensuring it arrives and is ready before it is
needed. The sending side will switch to using the new nonce once the
full 32768 messages of the current nonce have been used. At this
point, the sending side resets the offset to 0.

4. The receiving side must deal with the fact that UDP is unreliable
and unordered. The receiving side keeps track of the highest offset
seen for each established nonce. The condition to switching to the new
nonce it has received via the TCP connection is: the highest offset
seen for the current nonce must be above 24576 (75% of the available
range) _and_ the current message just received must have an offset
less than 8192 (25% of the available range) _and_ the new offset must
be less than 8192 messages *ahead* of the highest seen offset so far
(assuming modulo 32768). If these conditions are met, the highest
offset is set to the offset of the current message and the new nonce
is used. If those conditions are not met, the receiving side continues
with the current nonce (according to the rules below), updating the
highest offset seen as appropriate. Thus in order for the nonce to not
be updated incorrectly would require a loss of at least 8192 messages.

5. When the receiving side switches to the new nonce, it does not
discard the old nonce. If the highest offset seen is below 8192 (25% of
the range) _and_ the current message offset is above 24576 (75% of the
range), _and_ the current message offset is less than 8192 behind the
highest seen nonce (assuming modulo 32768), then the current message is
decoded with the old nonce.

6. In the remaining cases, the current message is only decoded if its
offset is within 8192 either side of the highest seen offset.

7. To avoid replay attacks, the receiving side keeps a set of which
offsets have been used with the current and previous nonce. If the
offset doesn't exist in the set, and the message can be correctly
decoded, the offset is added to the relevant set (thus we avoid
poisoning attacks). If the offset already exists in the set, or the
message cannot be correctly decoded, the message is not processed
further.

### Further reading
More details on the inner workings of weave can be found in the
[architecture documentation](https://github.com/zettio/weave/blob/master/docs/architecture.txt).
