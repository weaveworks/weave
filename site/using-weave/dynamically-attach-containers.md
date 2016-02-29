---
title: Dynamically Attaching and Detaching Applications
layout: default
---


When containers may not know the network to which they will be attached, Weave enables you to dynamically attach and detach containers to and from a given network, even when a container is already running. 

To illustrate these scenarios, imagine a netcat service running in a container on $host1 and you need to attach it to another subnet. To attach the netcat service container to a given subnet run: 

    host1$ C=$(docker run -e WEAVE_CIDR=none -dti ubuntu)
    host1$ weave attach $C
    10.2.1.3

Where, 

 *  `C=$(Docker run -e WEAVE_CIDR=none -dti ubuntu)` is a variable for the subnet on which to attach
 *  `weave attach` – the Weave command to attach to the specified subnet, which takes the variable for the subnet
 *  `10.2.1.3` - the allocated IP address output by `weave attach` and in this case, represents the default subnet

>>Note If you are using the Weave Docker API proxy, it will have modified `DOCKER_HOST` to point to the proxy and therefore you will have to pass `-e WEAVE_CIDR=none` to start a container that _doesn't_ get automatically attached to the weave network for the purposes of this example.

###Dynamically Detaching Containers

A container can be detached from a subnet, by using the `weave detach` command:

    host1$ weave detach $C
    10.2.1.3

You can also detach a container from one network and then attach it to a different one:

    host1$ weave detach net:default $C
    10.2.1.3
    host1$ weave attach net:10.2.2.0/24 $C
    10.2.2.3

or, attach a container to multiple application networks, effectively sharing the same container between applications:

    host1$ weave attach net:default
    10.2.1.3
    host1$ weave attach net:10.2.2.0/24
    10.2.2.3

Finally, multiple addresses can be attached or detached using a single command:

    host1$ weave attach net:default net:10.2.2.0/24 net:10.2.3.0/24 $C
    10.2.1.3 10.2.2.3 10.2.3.1
    host1$ weave detach net:default net:10.2.2.0/24 net:10.2.3.0/24 $C
    10.2.1.3 10.2.2.3 10.2.3.1

>>**Important!** Any addresses that were dynamically attached will not be re-attached if the container restarts.

**See Also**

 * [Adding and Removing Hosts Dynamically](/site/using-weave/finding-adding-hosts-dynamically.md)
