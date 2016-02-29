---
title: Isolating Applications on a Weave Network
layout: default
---

In some instances, you may need to run applications on the same container network, but at the same time keep the applications isolated from one another.
 
To do this, you need to configure Weave's IP allocator to manage multiple subnets:

~~~bash
    host1$ weave launch --ipalloc-range 10.2.0.0/16 --ipalloc-default-subnet 10.2.1.0/24
    host1$ eval $(weave env)
    host2$ weave launch --ipalloc-range 10.2.0.0/16 --ipalloc-default-subnet 10.2.1.0/24 $HOST1
    host2$ eval $(weave env)
~~~

This delegates the entire 10.2.0.0/16 subnet to Weave, and instructs it to allocate from 10.2.1.0/24 within that if no specific subnet is specified. 

Now you can launch two containers onto the default subnet:

~~~bash
    host1$ docker run --name a1 -ti ubuntu
    host2$ docker run --name a2 -ti ubuntu
~~~

And then launch several more containers onto a different subnet:

~~~bash
    host1$ docker run -e WEAVE_CIDR=net:10.2.2.0/24 --name b1 -ti ubuntu
    host2$ docker run -e WEAVE_CIDR=net:10.2.2.0.24 --name b2 -ti ubuntu
~~~

Ping the containers from each subnet to confirm that they can communicate with each other, but not to the containers of our first subnet:

~~~bash
    root@b1:/# ping -c 1 -q b2
    PING b2.weave.local (10.2.2.128) 56(84) bytes of data.
    --- b2.weave.local ping statistics ---
    1 packets transmitted, 1 received, 0% packet loss, time 0ms
    rtt min/avg/max/mdev = 1.338/1.338/1.338/0.000 ms
~~~

~~~bash
    root@b1:/# ping -c 1 -q a1
    PING a1.weave.local (10.2.1.2) 56(84) bytes of data.
    --- a1.weave.local ping statistics ---
    1 packets transmitted, 0 received, 100% packet loss, time 0ms
~~~

~~~bash
    root@b1:/# ping -c 1 -q a2
    PING a2.weave.local (10.2.1.130) 56(84) bytes of data.
    --- a2.weave.local ping statistics ---
    1 packets transmitted, 0 received, 100% packet loss, time 0ms
~~~

###Attaching Containers to Multiple Subnets and Isolating Applications

A container can also be attached to multiple subnets by using the following command line arguments with `docker run`:

~~~bash
    host1$ docker run -e WEAVE_CIDR="net:default net:10.2.2.0/24" -ti ubuntu
~~~

Where,

 *`net:default` is used to request the allocation of an address from the default subnet in addition to one from an explicitly specified range.

>>Note: By default Docker permits communication between containers on the same host, via their Docker-assigned IP addresses. For complete isolation between application containers, that feature _must_ to be disabled by [setting `--icc=false`](https://docs.Docker.com/engine/userguide/networking/default_network/container-communication/#communication-between-containers) in the Docker daemon configuration. 

>>**Important:!** Containers must not be allowed to capture and inject raw network packets. This can be prevented by starting the containers with the `--cap-drop net_raw` option.

**See Also**

 * [Dynamically Attaching and Detaching Applications](/site/using-weave/dynamically-attach-containers.md)
 * [Automatic Allocation Across Multiple Subnets](/site/ipam/allocation-multi-ipam.md)
