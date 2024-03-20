---
title: Concepts in Weave Net
menu_order: 20
search_type: Documentation
---

The section contains topics about how Weave Net works as well as providing a technical deep dive on how Weave Net implements fast datapath, and encryption.

* [Understanding Weave Net]({{ '/concepts/how-it-works' | relative_url }})
* [Weave Net Router Sleeve Encapsulation]({{ '/concepts/router-encapsulation' | relative_url }})
* [How Weave Net Interprets Network Topology]({{ '/concepts/network-topology' | relative_url }})
     * [Communicating Topology Among Peers]({{ '/concepts/network-topology#topology' | relative_url }})
     * [How Messages are Formed]({{ '/concepts/network-topology#messages' | relative_url }})
     * [Removing Peers]({{ '/concepts/network-topology#removing-peers' | relative_url }})
     * [What Happens When The Topology is Out of Date?]({{ '/concepts/network-topology#out-of-date-topology' | relative_url }})
* [Fast Datapath & Weave Net]({{ '/concepts/fastdp-how-it-works' | relative_url }})
* [IP Addresses, Routes and Networks]({{ '/concepts/ip-addresses' | relative_url }})
* [Encryption and Weave Net]({{ '/concepts/encryption' | relative_url }})
     * [How Weave Net Implements Encryption]({{ '/concepts/encryption-implementation' | relative_url }})
          * [Establishing the Emphemeral Session Key]({{ '/concepts/network-topology#ephemeral-key' | relative_url }})
          * [Key Generation and the Linux CSPRIN]({{ '/concepts/network-topology#cspring' | relative_url }})
          * [Encrypting and Decrypting TCP Messages]({{ '/concepts/network-topology#tcp' | relative_url }})
          * [Encrypting and Decrypting UDP Messages]({{ '/concepts/network-topology#udp' | relative_url }})
