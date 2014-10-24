---
title: Troubleshooting Weave
layout: default
---

## Troubleshooting

Make sure you are running the latest version - you can download it
with

    docker pull zettio/weave:latest

Check the weave container logs with

    docker logs weave

A reasonable amount of information, and all errors, get logged there.

The log verbosity can be increased by supplying the `-debug` flag when
launching weave. Be warned, this will log information on a per-packet
basis, so can produce a lot of output.

Another useful debugging technique is to attach standard packet
capture and analysis tools, such as tcpdump and wireshark, to the
`weave` network bridge on the host.

<hr/>

To get a list of all the containers running on this host that are
connected to the weave network:

    weave ps

This produces output like:

    5245643870f1 10.0.5.1/16
    e32a7d37a93a 10.0.8.3/24
    caa1d4ee2570 10.0.1.1/24 10.0.2.1/24

giving, for each running container attached, the container ID then the
list of IP address/routing prefix length ([CIDR
notation](http://en.wikipedia.org/wiki/Classless_Inter-Domain_Routing))
assigned on the weave network.

<hr/>

One can ask a weave router to report its status with

    weave status

This produces output like:

````
Local identity is 7a:f4:56:87:76:3b
Sniffing traffic on &{39 65535 ethwe ae:e3:07:9c:8c:d4 up|broadcast|multicast}
MACs:
Peers:
Peer 7a:16:dd:5b:83:de (v31) (UID 13151318985609435078)
   -> 7a:f4:56:87:76:3b [37.157.33.76:7195]
Peer 7a:f4:56:87:76:3b (v1) (UID 6913268221365110570)
   -> 7a:16:dd:5b:83:de [191.235.147.190:6783]
Topology:
unicast:
7a:f4:56:87:76:3b -> 00:00:00:00:00:00
7a:16:dd:5b:83:de -> 7a:16:dd:5b:83:de
broadcast:
7a:f4:56:87:76:3b -> [7a:16:dd:5b:83:de]
7a:16:dd:5b:83:de -> []
Reconnects:
192.168.32.1:6783 (next try at 2014-10-23 16:39:50.585932102 +0000 UTC)
````

The terms used here are explained further at [how it
works](http://zettio.github.io/weave/how-it-works.html).

A 'peer' on the weave network is a weave router; one per host.  Each
peer has a name, which will tend to remain the same over restarts, and
a unique identifier (UID) which will be different each time it is run.
These are opaque identifiers as far as the program is concerned,
although the name defaults to a MAC address.

The 'sniffing traffic' line shows details of the virtual ethernet
interface that weave is using to receive packets on the local
machine.

Then comes a list of all peers known to this router, including itself.
Each peer is listed with its name, a version number (incremented on
each reconnect) and the UID.  Then each line beginning `->` shows
another peer that it is connected to, with the IP address and port
number of the connection. In the above example, the local router has
connected to its peer using address 191.235.147.190:6783, and its peer
sees the same connection as coming from 37.157.33.76:7195.

After that comes the topology information used to decide how to route packets between peers - see  the [architecture documentation](https://raw.githubusercontent.com/zettio/weave/master/docs/architecture.txt) for full explanation.

Finally 'Reconnects', which shows peers that this router is aware of,
but is not currently connected to.  Each line will contain some
information about whether it is attempting to connect or will wait for
a while before connecting again.

<hr/>

To stop weave, run

    weave stop

Note that this leaves the local application container network intact;
containers on the local host can continue to communicate, whereas
communication with containers on different hosts, as well as service
export/import, is disrupted but resumes when weave is relaunched.

To stop weave and completely remove all traces of the weave network on
the local host, run

    weave reset

Any running application containers will permanently lose connectivity
with the weave network and have to be restarted in order to
re-connect.

### Reboots

When a host reboots, docker's default behaviour is to restart any
containers that were running. Since weave relies on special network
configuration outside of the containers, the weave network will not
function in this state.

To remedy this, stop and re-launch the weave container, and re-attach
the application containers with `weave attach`.

For a more permanent solution,
[disable Docker's auto-restart feature](https://docs.docker.com/articles/host_integration/)
and create appropriate startup scripts to launch weave and run
application containers from your favourite process manager.
