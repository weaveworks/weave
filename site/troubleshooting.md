---
title: Troubleshooting Weave
layout: default
---

## Troubleshooting

Check what version of weave you are running with

    weave version

If it is not the latest version, as shown in the list of
[releases](https://github.com/weaveworks/weave/releases), then it is
highly recommended that you upgrade by following the
[installation instructions](https://github.com/weaveworks/weave#installation).

Check the weave container logs with

    docker logs weave

A reasonable amount of information, and all errors, get logged there.

The log verbosity can be increased by supplying the `-debug` flag when
launching weave. Be warned, this will log information on a per-packet
basis, so can produce a lot of output.

Another useful debugging technique is to attach standard packet
capture and analysis tools, such as tcpdump and wireshark, to the
`weave` network bridge on the host.

One can ask a weave router for a [status report](#status-report) or
for a [list of attached containers](#list-attached-containers).

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

### <a name="status-report"></a>Status report

The command

    weave status

reports on the current status of the weave router and DNS.

This produces output like:

````
weave router 0.7.0
Encryption off
Our name is 7a:f4:56:87:76:3b(weave01)
Sniffing traffic on &{39 65535 ethwe ae:e3:07:9c:8c:d4 up|broadcast|multicast}
MACs:
ba:8c:b9:dc:e1:c9 -> 7a:f4:56:87:76:3b(weave01) (2014-10-23 16:39:19.482338935 +0000 UTC)
ce:15:34:a9:b5:6d -> 7a:f4:56:87:76:3b(weave01) (2014-10-23 16:39:28.257103595 +0000 UTC)
7a:61:a2:49:4b:91 -> 7a:f4:56:87:76:3b(weave01) (2014-10-23 16:39:27.482970752 +0000 UTC)
9e:95:0c:54:8e:39 -> 7a:16:dd:5b:83:de(weave02) (2014-10-23 16:39:28.795601325 +0000 UTC)
72:5f:a4:60:e5:ce -> 7a:16:dd:5b:83:de(weave02) (2014-10-23 16:39:29.575995255 +0000 UTC)
Peers:
7a:16:dd:5b:83:de(weave02) (v31) (UID 13151318985609435078)
   -> 7a:f4:56:87:76:3b(weave01) [37.157.33.76:7195]
7a:f4:56:87:76:3b(weave01) (v1) (UID 6913268221365110570)
   -> 7a:16:dd:5b:83:de(weave02) [191.235.147.190:6783]
Routes:
unicast:
7a:f4:56:87:76:3b -> 00:00:00:00:00:00
7a:16:dd:5b:83:de -> 7a:16:dd:5b:83:de
broadcast:
7a:f4:56:87:76:3b -> [7a:16:dd:5b:83:de]
7a:16:dd:5b:83:de -> []
Reconnects:
->[192.168.32.1:6783] (dial tcp4 192.168.32.1:6783: connection timed out) next try at 2014-10-23 16:39:50.585932102 +0000 UTC
````

The terms used here are explained further at
[how it works](how-it-works.html).

The 'Our name' line identifies the local weave router as a peer in the
weave network, displaying the peer name followed by the peer's nickname
in parenthesis. The nickname defaults to the name of the host on which
the weave container was launched; if desired it can be overriden by
supplying the `-nickname` argument to `weave launch`.

The 'Sniffing traffic' line shows details of the virtual ethernet
interface that weave is using to receive packets on the local
machine.

The 'MACs' section lists all MAC addresses known to this router. These
identify containers in the weave network, as well as points for
[host network integration](features.html#host-network-integration). For
each MAC the list shows the peer they reside on, and the time when the
router last saw some traffic from that MAC. The router forgets
addresses which are inactive for longer than 10 minutes.

The 'Peers' section lists all peers known to this router, including
itself.  Each peer is shown with its name, nickname, version number
(incremented on each reconnect) and the UID.  Then each line
beginning `->` shows another peer that it is connected to, with the
IP address and port number of the connection. In the above example,
the local router has connected to its peer using address
191.235.147.190:6783, and its peer sees the same connection as coming
from 37.157.33.76:7195.

The 'Routes' section summarised the information for deciding how to
route packets between peers, which is mostly of interest when the
weave network is not fully connected.  See the
[architecture documentation](https://raw.githubusercontent.com/weaveworks/weave/master/docs/architecture.txt)
for a full explanation.

The 'Reconnects' section lists peers that this router is aware of, but is
not currently connected to.  Each line contains some information about
what went wrong the last time; whether it is attempting to connect or
is waiting for a while before connecting again.

### <a name="list-attached-containers"></a>List attached containers

    weave ps

Produces a list of all the containers running on this host that are
connected to the weave network, like this:

    weave:expose 7a:c4:8b:a1:e6:ad 10.2.5.2/24
    b07565b06c53 ae:e3:07:9c:8c:d4
    5245643870f1 ce:15:34:a9:b5:6d 10.2.5.1/24
    e32a7d37a93a 7a:61:a2:49:4b:91 10.2.8.3/24
    caa1d4ee2570 ba:8c:b9:dc:e1:c9 10.2.1.1/24 10.2.2.1/24

On each line are the container ID, its MAC address, then the list of
IP address/routing prefix length ([CIDR
notation](http://en.wikipedia.org/wiki/Classless_Inter-Domain_Routing))
assigned on the weave network. The special container name `weave:expose`
displays the weave bridge MAC and any IP addresses added to it via the
`weave expose` command.

You can also supply a list of container IDs/names to `weave ps`, like this:

    $ sudo weave ps able baker
    able ce:15:34:a9:b5:6d 10.2.5.1/24
    baker 7a:61:a2:49:4b:91 10.2.8.3/24

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

### <a name="snapshots"></a>Snapshot Releases

We sometimes publish snapshot releases, to provide previews of new
features, assist in validation of bug fixes, etc. One can install the
latest snapshot release with

    sudo wget -O /usr/local/bin/weave \
      https://raw.githubusercontent.com/weaveworks/weave/master/weave
    sudo chmod a+x /usr/local/bin/weave
    sudo weave setup

Snapshot releases report the script version as "(unreleased version)",
and the container image versions as git hashes.
