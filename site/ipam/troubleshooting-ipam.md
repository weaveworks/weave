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
     Consensus: waiting(quorum: 2, known: 0)
         Range: 10.32.0.0-10.47.255.255
 DefaultSubnet: 10.32.0.0/12

...
````

The first section covers the router; see the [troubleshooting
guide](/site/troubleshooting.md#weave-status) for full details.

The 'Service: ipam' section displays the consensus state as well as
the total allocation range and default subnet.