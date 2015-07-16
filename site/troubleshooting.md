---
title: Troubleshooting Weave
layout: default
---

# Troubleshooting Weave

 * [Overall status](#weave-status)
 * [List connections](#weave-status-connections)
 * [List peers](#weave-status-peers)
 * [List DNS entries](#weave-status-dns)
 * [JSON report](#weave-report)
 * [List attached containers](#list-attached-containers)
 * [Snapshot releases](#snapshot-releases)

Check what version of weave you are running with

    weave version

If it is not the latest version, as shown in the list of
[releases](https://github.com/weaveworks/weave/releases), then it is
highly recommended that you upgrade by following the
[installation instructions](https://github.com/weaveworks/weave#installation).

Check the weave container logs with

    docker logs weave

A reasonable amount of information, and all errors, get logged there.

The log verbosity can be increased by supplying the
`--log-level=debug` option when launching weave. To log information on
a per-packet basis use `--pktdebug` - be warned, this can produce a
lot of output.

Another useful debugging technique is to attach standard packet
capture and analysis tools, such as tcpdump and wireshark, to the
`weave` network bridge on the host.

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

### <a name="weave-status"></a>Overall status

A status summary can be obtained with `weave status`:

````
$ weave status
       Version: 1.1.0

       Service: router
      Protocol: weave 1..2
          Name: 4a:0f:f6:ec:1c:93(host1)
    Encryption: disabled
 PeerDiscovery: enabled
       Targets: [192.168.48.14 192.168.48.15]
   Connections: 5 (1 established, 1 pending, 1 retrying, 1 failed, 1 connecting)
         Peers: 3 (with 5 established, 1 pending connections betweeen them)

       Service: ipam
     Consensus: achieved
         Range: [10.32.0.0-10.48.0.0)
 DefaultSubnet: 10.32.0.0/12

       Service: dns
        Domain: weave.local.
           TTL: 1
       Entries: 9

       Service: proxy
       Address: tcp://127.0.0.1:12375
````

The terms used here are explained further at
[how it works](how-it-works.html).

The 'Version' line shows the weave version.

The 'Protocol' line indicates the weave router's inter-peer
communication protocol name and supported versions (min..max).

The 'Name' line identifies the local weave router as a peer in the
weave network. The nickname shown in parentheses defaults to the name
of the host on which the weave container was launched; if desired it
can be overriden by supplying the `--nickname` argument to `weave
launch`.

The 'Encryption' line indicates whether
[encryption](features.html#security) is in use for communication
between peers.

The 'PeerDiscovery' line indicates whether
[automatic peer discovery](features.html#dynamic-topologies) is
enabled (which is the default).

'Targets' is the number of hosts that the local weave router has been
asked to connect to in `weave launch` and `weave connect`. The
complete list can be obtained with `weave status targets`.

'Connections' shows the total number connections between the local weave
router and other peers, and a break down of that figure by connection
state. Further details are available with
[`weave status connections`](#weave-status-connections).

'Peers' shows the total number of peers in the network, and the number of
connections between them. Further details are available with
[`weave status peers`](#weave-status-peers).

There are further sections for the [IP address
allocator](ipam.html#troubleshooting),
[weaveDNS](weavedns.html#troubleshooting), and [Weave Docker API
Proxy](proxy.html#troubleshooting).

### <a name="weave-status-connections"></a>List connections

Detailed information on the local weave router's connections can be
obtained with `weave status connections`:

````
$ weave status connections
<- 192.168.48.12:33866   established 7e:21:4a:70:2f:45(host2)
<- 192.168.48.13:60773   pending     7e:ae:cd:d5:23:8d(host3)
-> 192.168.48.14:6783    retrying    dial tcp4 192.168.48.14:6783: no route to host
-> 192.168.48.15:6783    failed      dial tcp4 192.168.48.15:6783: no route to host, retry: 2015-08-06 18:55:38.246910357 +0000 UTC
-> 192.168.48.16:6783    connecting
````

The columns are as follows:

 * Connection origination direction (`->` for outbound, `<-` for
   inbound)
 * Remote TCP address
 * Status
    * `connecting` - first connection attempt in progress
    * `failed` - TCP connection or UDP heartbeat failed
    * `retrying` - retry of a previously failed connection attempt in
      progress; reason for previous failure follows
    * `pending` - TCP connection up, waiting for confirmation of
       UDP heartbeat
    * `established` - TCP connection and corresponding UDP path are up
 * Info - the remote peer name and nickname for (un)established
   connections, the failure reason for failed and retrying connection

### <a name="weave-status-peers"></a>List peers

Detailed information on peers can be obtained with `weave status
peers`:

````
$ weave status peers
ce:31:e0:06:45:1a(host1)
   <- 192.168.48.12:39634   ea:2d:b2:e6:e4:f5(host2)         established
   <- 192.168.48.13:49619   ee:38:33:a7:d9:71(host3)         established
ea:2d:b2:e6:e4:f5(host2)
   -> 192.168.48.11:6783    ce:31:e0:06:45:1a(host1)         established
   <- 192.168.48.13:58181   ee:38:33:a7:d9:71(host3)         established
ee:38:33:a7:d9:71(host3)
   -> 192.168.48.12:6783    ea:2d:b2:e6:e4:f5(host2)         established
   -> 192.168.48.11:6783    ce:31:e0:06:45:1a(host1)         established
````

This lists all peers known to this router, including itself.  Each
peer is shown with its name and nickname, then each line thereafter
shows another peer that it is connected to, with the direction, IP
address and port number of the connection.  In the above example,
`host3` has connected to `host1` at `192.168.48.11:6783`; `host1` sees
the `host3` end of the same connection as `192.168.48.13:49619`.

### <a name="weave-status-dns"></a>List DNS entries

Detailed information on DNS registrations can be obtained with `weave
status dns`:

````
$ weave status dns
one          10.32.0.1       eebd81120ee4 4a:0f:f6:ec:1c:93
one          10.43.255.255   4fcec78d2a9b 66:c4:47:c6:65:bf
one          10.40.0.0       bab69d305cba ba:98:d0:37:4f:1c
three        10.32.0.3       7615b6537f74 4a:0f:f6:ec:1c:93
three        10.44.0.1       c0b39dc52f8d 66:c4:47:c6:65:bf
three        10.40.0.2       8a9c2e2ef00f ba:98:d0:37:4f:1c
two          10.32.0.2       83689b8f34e0 4a:0f:f6:ec:1c:93
two          10.44.0.0       7edc306cb668 66:c4:47:c6:65:bf
two          10.40.0.1       68a5e9c2641b ba:98:d0:37:4f:1c
````

The columns are as follows:

 * Unqualified hostname
 * IPv4 address
 * Registering entity identifier (typically a container ID)
 * Name of peer from which the registration originates

### <a name="weave-report"></a>JSON report

    $ weave report

Produces a comprehensive dump of the internal state of the router,
IPAM and DNS services in JSON format, including all the information
available from the `weave status` commands. You can also supply a
Golang text template to `weave report` in a similar fashion to `docker
inspect`:

    $ weave report -f '{{{.DNS.Domain}}'
    weave.local.

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

    $ weave ps able baker
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

    sudo curl -L git.io/weave-snapshot -o /usr/local/bin/weave
    sudo chmod a+x /usr/local/bin/weave
    weave setup

Snapshot releases report the script version as "(unreleased version)",
and the container image versions as git hashes.
