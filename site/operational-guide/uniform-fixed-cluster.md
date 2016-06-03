---
title: Uniform Fixed Clusters
menu_order: 30
---

This scenario describes a production deployment of a fixed number of N
nodes (N=1 in the simplest case). A uniform fixed cluster has the
following characteristics:

* Recovers automatically from reboots and partitions.
* All peers have identical configuration.
* There is a controlled process for adding or removing nodes, however
  the end user is responsible for ensuring that only one instance of
  the process is in-flight at a time. Whilst it is possible to
  automate, the potentially-blocking `weave prime` operation and the
  need for global serialisation make it non-trivial; it is however
  relatively straightforward for a human to provide the necessary
  guarantees and exception handling manually, so this scenario is best
  suited to deployments which change size infrequently as a planned
  maintenance event.

### Bootstrap

On each initial peer, at boot, via
[systemd](/site/installing-weave/systemd.md):

    weave launch --no-restart $PEERS

* `--no-restart` disables the Docker restart policy as this will be
  handled by systemd.
* `$PEERS` is obtained from `/etc/sysconfig/weave` as described in the
  linked systemd documentation. For convenience, this may contain the
  address of the peer which is being launched, so that you don't have
  to compute separate lists of 'other' peers tailored to each peer -
  just supply the same complete list of peer addresses to every peer.

Then on _any_ peer (note you can run this safely on more than one or
even all peers, but it's only strictly necessary to run it on one of
them):

    weave prime

to force consensus - once this completes successfully, IP address
allocations can proceed under partition and it is safe to add new
peers. If this command blocks it means that there is some issue (such
as a network partition or failed peers) that is preventing a quorum
from being reached – you will need to [address
that](/site/troubleshooting.md) before moving on.

### Add a Peer

On new peer, at boot, via
[systemd](/site/installing-weave/systemd.md):

    weave launch --no-restart $PEERS

Where `$PEERS` is the new peer plus all other peers in the network,
initial and subsequently added, which have not been explicitly
removed. It should include peers which are temporarily offline or
stopped.

For maximum robustness, you should then distribute an updated
`/etc/sysconfig/weave` file including the new peer to all existing
peers.

### Remove a Peer

On peer to be removed:

    weave reset

Then distribute an updated `/etc/sysconfig/weave` to the remaining
peers, omitting the removed peer from `$PEERS`.

On each remaining peer:

    weave forget <removed peer>

This final step is not mandatory, but it will eliminate log noise and
spurious network traffic by stopping reconnection attempts.
