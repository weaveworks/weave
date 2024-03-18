---
title: Concepts in Weave Net
menu_order: 20
search_type: Documentation
---

The section contains topics about how Weave Net works as well as providing a technical deep dive on how Weave Net implements fast datapath, and encryption.

* [Understanding Weave Net](/site/concepts/how-it-works.md)
* [Weave Net Router Sleeve Encapsulation](/site/concepts/router-encapsulation.md)
* [How Weave Net Interprets Network Topology](/site/concepts/network-topology.md)
     * [Communicating Topology Among Peers](https://weave.works/docs/net/latest/concepts/network-topology/#topology)
     * [How Messages are Formed](https://weave.works/docs/net/latest/concepts/network-topology/#messages)
     * [Removing Peers](https://weave.works/docs/net/latest/concepts/network-topology/#removing-peers)
     * [What Happens When The Topology is Out of Date?](https://weave.works/docs/net/latest/concepts/network-topology/#out-of-date-topology)
* [Fast Datapath & Weave Net](/site/concepts/fastdp-how-it-works.md)
* [IP Addresses, Routes and Networks](/site/concepts/ip-addresses.md)
* [Encryption and Weave Net](/site/concepts/encryption.md)
     * [How Weave Net Implements Encryption](/site/concepts/encryption-implementation.md)
          * [Establishing the Emphemeral Session Key](https://weave.works/docs/net/latest/concepts/network-topology/#ephemeral-key)
          * [Key Generation and the Linux CSPRIN](https://weave.works/docs/net/latest/concepts/network-topology/#cspring)
          * [Encrypting and Decrypting TCP Messages](https://weave.works/docs/net/latest/concepts/network-topology/#tcp)
          * [Encrypting and Decrypting UDP Messages](https://weave.works/docs/net/latest/concepts/network-topology/#udp)
