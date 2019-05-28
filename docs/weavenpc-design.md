# Overview

# ipsets

The policy controller maintains a number of ipsets which are
subsequently referred to by the iptables rules used to effect network
policy specifications. These ipsets are created, modified and
destroyed automatically in response to Pod, Namespace and
NetworkPolicy object updates from the k8s API server:

* A `hash:ip` set per namespace, containing the IP addresses of all
  pods in that namespace for which default ingress is allowed
* A `hash:ip` set per namespace, containing the IP addresses of all
  pods in that namespace for which default egress is allowed  
* A `list:set` per distinct (across all network policies in all
  namespaces) namespace selector mentioned in a network policy,
  containing the names of any of the above hash:ip sets whose
  corresponding namespace labels match the selector
* A `hash:ip` set for each distinct (within the scope of the
  containing network policy's namespace) pod selector mentioned in a
  network policy, containing the IP addresses of all pods in the
  namespace whose labels match that selector
* A `hash:net` set for each distinct (within the scope of the
  containing network policy's namespace) `except` list of CIDR's mentioned in
  the network policies

IPsets are implemented by the kernel module `xt_set`, without which
weave-npc will not work.

ipset names are generated deterministically from a string
representation of the corresponding label selector. Because ipset
names are limited to 31 characters in length, this is done by taking a
SHA hash of the selector string and then printing that out as a base
85 string with a "weave-" prefix e.g.:

    weave-k?Z;25^M}|1s7P3|H9i;*;MhG

Because pod selectors are scoped to a namespace, we need to make sure
that if the same selector definition is used in different namespaces
that we maintain distinct ipsets. Consequently, for such selectors the
namespace name is prepended to the label selector string before
hashing to avoid clashes.

# iptables chains

The policy controller maintains several iptables chains in response to
changes to pods, namespaces and network policies. 

## Static `WEAVE-NPC` chain

`WEAVE-NPC` chain contains static rules to ACCEPT traffic that is `RELATED,ESTABLISHED`
and run `NEW` traffic through `WEAVE-NPC-DEFAULT` followed by `WEAVE-NPC-INGRESS` chains 

Static configuration:

```
iptables -A WEAVE-NPC -m state --state RELATED,ESTABLISHED -j ACCEPT
iptables -A WEAVE-NPC -m state --state NEW -j WEAVE-NPC-DEFAULT
iptables -A WEAVE-NPC -m state --state NEW -j WEAVE-NPC-INGRESS
```

## Dynamically maintained `WEAVE-NPC-DEFAULT` chain

The policy controller maintains a rule in this chain for every
namespace whose ingress isolation policy is `DefaultAllow`. The
purpose of this rule is simply to ACCEPT any traffic destined for such
namespaces before it reaches the ingress chain.

```
iptables -A WEAVE-NPC-DEFAULT -m set --match-set $NSIPSET dst -j ACCEPT
```

## Dynamically maintained `WEAVE-NPC-INGRESS` chain

For each namespace network policy ingress rule peer/port combination:

```
iptables -A WEAVE-NPC-INGRESS -p $PROTO [-m set --match-set $SRCSET] -m set --match-set $DSTSET --dport $DPORT -j ACCEPT
```

## Static `WEAVE-NPC-EGRESS` chain

`WEAVE-NPC-EGRESS` chain contains static rules to ACCEPT traffic that is `RELATED,ESTABLISHED`
and run `NEW` traffic through `WEAVE-NPC-EGRESS-DEFAULT` followed by `WEAVE-NPC-EGRESS-CUSTOM` chains 

Static configuration:

```
iptables -A WEAVE-NPC-EGRESS -m state --state RELATED,ESTABLISHED -j ACCEPT
iptables -A WEAVE-NPC-EGRESS -m state --state NEW -j WEAVE-NPC-EGRESS-DEFAULT
iptables -A WEAVE-NPC-EGRESS -m state --state NEW -m mark ! --mark 0x40000/0x40000 -j WEAVE-NPC-EGRESS-CUSTOM
iptables -A WEAVE-NPC-EGRESS -m mark ! --mark 0x40000/0x40000 -j DROP
```

## Dynamically maintained `WEAVE-NPC-EGRESS-DEFAULT` chain

The policy controller maintains a rule in this chain for every
namespace whose egress isolation policy is `DefaultAllow`. The
purpose of this rule is simply to ACCEPT any traffic originating from such namespace before it reaches the egress chain.

```
iptables -A WEAVE-NPC-EGRESS-DEFAULT -m set --match-set $NSIPSET src -j WEAVE-NPC-EGRESS-ACCEPT
iptables -A WEAVE-NPC-EGRESS-DEFAULT -m set --match-set $NSIPSET src -j RETURN
```

## Static `WEAVE-NPC-EGRESS-ACCEPT` chain

`WEAVE-NPC-EGRESS-ACCEPTS` chain contains static rules to mark traffic

Static configuration:

```
iptables -A WEAVE-NPC-EGRESS-ACCEPT -j MARK --set-xmark 0x40000/0x40000
```

## Dynamically maintained `WEAVE-NPC-EGRESS-CUSTOM` chain

For each namespace network policy egress rule peer/port combination:

```
iptables -A WEAVE-NPC-EGRESS-CUSTOM -p $PROTO [-m set --match-set $SRCSET] -m set --match-set $DSTSET --dport $DPORT -j ACCEPT
```


# Steering traffic into the policy engine

To direct traffic into the policy engine:

```
iptables -A INPUT -i weave -j WEAVE-NPC-EGRESS
iptables -A FORWARD -i weave -j WEAVE-NPC-EGRESS
iptables -A FORWARD -o weave -j WEAVE-NPC
```

Note this only affects traffic which egresses the bridge on a physical
port which is not the Weave Net router - in other words, it is
destined for an application container veth. The following traffic is
affected:

* Traffic bridged between local application containers
* Traffic bridged from the router to a local application container
* Traffic originating from the internet destined for nodeports - this
  is routed via the FORWARD chain to a container pod IP after DNAT

The following traffic is NOT affected:

* Traffic bridged from a local application container to the router
* Traffic originating from processes in the host network namespace
  (e.g. kubelet health checks)
* Traffic routed from an application container to the internet

The above mechanism relies on the kernel module `br_netfilter` being
loaded and enabled via `/proc/sys/net/bridge/bridge-nf-call-iptables`.

See these resources for helpful context:

* http://ebtables.netfilter.org/br_fw_ia/br_fw_ia.html
* https://commons.wikimedia.org/wiki/File:Netfilter-packet-flow.svg
