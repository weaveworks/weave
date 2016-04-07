---
title: Troubleshooting the IP Allocator
layout: default
---


The command

    weave status

reports on the current status of the weave router and IP allocator:

````
...
       Service: ipam
        Status: awaiting consensus (quorum: 2, known: 0)
         Range: 10.32.0.0-10.47.255.255
 DefaultSubnet: 10.32.0.0/12
...
````

The first section covers the router; see the [troubleshooting
guide](/site/troubleshooting.md#weave-status) for full details.

The 'Service: ipam' section displays the consensus state as well as
the total allocation range and default subnet. Columns are as follows:

* 'Status' - allocator state
    * 'idle' - no allocation requests or claims have been made yet;
      consensus is deferred until then
    * 'awaiting consensus' - an attempt to achieve consensus is
      ongoing, triggered by an allocation or claim request;
      allocations will block.  This state persists until a quorum of
      peers are able to communicate amongst themselves successfully.
      The Status may also show '(observer)' instead of the quorum size,
      if this peer has been started with the `--observer` option.
    * 'ready' - consensus achieved; allocations proceed normally
    * 'waiting for IP range grant from peers' - peer has exhausted its
      agreed portion of the range and is waiting to be granted some
      more
    * 'all IP ranges owned by unreachable peers' - peer has exhausted
      its agreed portion of the range but cannot reach anyone to ask
      for more
* 'Range' - total allocation range set by `--ipalloc-range`
* 'DefaultSubnet' - default subnet set by `--ipalloc-default-subnet`
