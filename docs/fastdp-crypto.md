# Overview

At the high level, we use the ESP protocol (RFC 2406) in the Transport mode.
Each packet is encrypted with AES in GCM mode (RFC 4106), with 32 bytes key
and 4 bytes salt. This combo provides the following security properties:

* Data confidentiality.
* Data origin authentication.
* Integrity.
* Partial anti-replay service (partial - because we do not rotate keys which
  makes replay attacks possible when Sequence Number in ESP header overflows).
* Limited traffic flow confidentiality.

# Key derivation

For each connection direction a different AES-GCM key (and salt) is used. The keys
are derived by applying HKDF (RFC 5869) to `SessionKey` which is derived by Mesh
during the handshake. The current implementation does not do key rotations.

# SPI

The IPsec connection between two peers is identified by directional SPIs
formed by concatenating `fromPeerShortID` and `toPeerShortID` and setting
the MSB to make sure that SPI does not fall into 1-255 range which is reserved
by IANA.

# Implementation

The implementation is based on the kernel IP packet transformation framework
called XFRM. Unfortunately, docs are barely existing and the complexity of
the framework is high. The best resource I found is Chapter 10 in
"Linux Kernel Networking: Implementation and Theory" by Rami Rosen.

The kernel VXLAN driver does not set a dst port of a tunnel in the ip flow
descriptor, thus XFRM policy lookup cannot match a policy which includes
the dst port. This makes impossible to encrypt only tunneled traffic between
peers. To work around, we mark such outgoing packets with iptables and set
the same mark in the policy selector. Funnily enough, iptables_mangle module
eventually sets the missing dst port in the flow descriptor. The challenge
here is to pick a mark that it would not interfere with other networking
applications before OUTPUT'ing a packet. For example, Kubernetes by default
uses 1<<14 and 1<<15 marks. Additionally, such workaround brings
the requirement for at least the 4.2 kernel.

To establish IPsec between two peers, we need to do the following on each
peer:

1. Create (inbound) SA which determines how to process (decrypt) an
   incoming packet from the remote peer.
2. Create (outbound) SA for encrypting a packet destined to the remote
   peer.
3. Create XFRM policy which says what SA to apply for an outgoing packet.
4. Install iptables rules for marking the tunneled outbound traffic.

# MTU

In addition to the VXLAN overhead, the MTU calculation of Weave Net should
take into account the ESP overhead which is 34-37 bytes (encrypted Payload is 4
bytes aligned) and consists of:

* 4 bytes (SPI).
* 4 bytes (Sequence Number).
* 8 bytes (ESP IV).
* 1 byte (Pad Length).
* 1 byte (NextHeader).
* 16 bytes (ICV).
* 0-3 bytes (Padding).
