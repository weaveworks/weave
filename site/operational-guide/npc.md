---
title: Weave NPC
menu_order: 70
---

* ["Connection dropped by Weave NPC"](#connection-dropped)

##<a name="connection-dropped"></a>"Connection dropped by Weave NPC"

When using Weave together with [Kubernetes](http://kubernetes.io) and [Weave NPC](https://github.com/weaveworks/weave-npc), Weave's Kubernetes Network Policy Controller, `weave launch --expect-npc` sets up the two below default `iptables` rules, in case none of the NPC rules apply:

    LOG --log-prefix="Connection dropped by Weave NPC (see: bit.ly/2eNRyDF)"
    DROP

This can eventually generate kernel log messages like the below one:

    [12345.678901] Connection dropped by Weave NPC (see: bit.ly/2eNRyDF):=IN= OUT=eth0 SRC=xxx.xxx.xxx.xxx DST=yyy.yyy.yyy.yyy LEN=xx TOS=0x00 PREC=0x00 TTL=64 ID=1001 DF PROTO=ICMP TYPE=8 CODE=0 ID=1337 SEQ=1

If you ever see the above message and did not expect it, then:

* you must have enabled the `DefaultDeny` policy in NPC -- which is off by default, in which case NPC implements `DefaultAllow` and the packet never makes it to the above default rules as it is accepted by NPC much earlier;
* you did not configure a network policy allowing the traffic for this packet.

This could mean that either:

* you have misconfigured your network policies and something which should be allowed access is being blocked, in which case you may want to review your network policies to correct this, or
* something genuinely unauthorized is attempting access, in which case you may want to take action.
