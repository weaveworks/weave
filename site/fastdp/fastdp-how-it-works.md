---
title: How Fast Datapath Works
layout: default
---


Weave implements an overlay network between Docker hosts, where each packet is encapsulated in a tunnel protocol header and then sent to the destination host, where the header is removed. The Weave router is a user space process, which means that the packet follows a winding path in and out of the Linux kernel:

![Weave Net Encapsulation](/images/weave-net-encap1-1024x459.png)

Weave's Fast datapath uses the Linux kernel's [Open vSwitch datapath module](https://www.kernel.org/doc/Documentation/networking/openvswitch.txt). This module enables the Weave router to tell the kernel how to process packets:

![Weave Net Encapsulation](/images/weave-net-fdp1-1024x454.png)

Because Weave Net issues instructions directly to the kernel, the amount of packet data and also the context switches are decreased. In addition to this, `fast datapath` reduces CPU overhead and latency. The packet goes straight from your application to the kernel, where the Virtual Extensible Lan (VXLAN) header is added (the NIC does this if it offers VXLAN acceleration). VXLAN is an IETF standard UDP-based tunneling protocol that enable you to use common networking tools like [Wireshark](https://www.wireshark.org/) to inspect the tunneled packets.

![Weave Net Encapsulation](/images/weave-frame-encapsulation-178x300.png)

Prior to version 1.2, Weave Net used a custom encapsulation format. Fast data path uses VXLAN, and like Weave Net's custom encapsulation format, VXLAN is UDP-based, and therefore does not interfere with network infrastructure. 

>>Note:The required open vSwitch datapath (ODP) and VXLAN features are present in Linux kernel versions 3.12 and greater. If your kernel was built without the necessary modules Weave Net will fall back to the "user mode" packet path.



