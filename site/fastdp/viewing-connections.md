---
title:Viewing Connection Mode Fastdp or Sleeve
layout: default
---


Weave automatically uses the fastest datapath for every connection unless it encounters a situation that prevents it from working. To ensure that Weave can use the fast data path:

 * Avoid Network Address Translation (NAT) devices
 * Open UDP port 6784 (This is the port used by the Weave routers)
 * Ensure that `WEAVE_MTU` fits with the `MTU` of the intermediate network (see below)

The use of fast datapath is an automated connection-by-connection decision made by Weave, and because of this, you may end up with a mixture of connection tunnel types. If fast data path cannot be used for a connection, Weave falls back to the "user space" packet path. 

Once a Weave network is set up, you can query the connections using the `weave status connections` command:

~~~bash
$ weave status connections
<-192.168.122.25:43889  established fastdp a6:66:4f:a5:8a:11(ubuntu1204)
~~~

Where fastdp indicates that fast data path is being used on a connection. If fastdp is not shown, the field displays `sleeve` indicating Weave Net's fall-back encapsulation method:

~~~bash
$ weave status connections
<- 192.168.122.25:54782  established sleeve 8a:50:4c:23:11:ae(ubuntu1204)
~~~