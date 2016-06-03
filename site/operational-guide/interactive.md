---
title: Interactive Deployment
menu_order: 20
---

This pattern is recommended for exploration and evaluation only, as
the commands described herein are interactive and not readily amenable
to automation and configuration management. Nevertheless, the
resulting weave network will survive host reboots without the use of a
systemd unit as long as Docker is configured to start on boot.

### Bootstrap

On initial peer:

    weave launch

### Add a Peer

On new peer:

    weave launch <extant peers>

Where `<extant peers>` means all peers in the network, initial and
subsequently added, which have not been explicitly removed. It should
include peers which are temporarily offline or stopped.

You must then execute:

    weave prime

to ensure that the new peer has joined to the existing network; you
_must_ wait for this to complete successfully before moving on to add
further new peers. If this command blocks it means that there is some
issue (such as a network partition or failed peers) that is preventing
a quorum from being reached - you will need to [address
that](/site/troubleshooting.md) before moving on.

### Stop a Peer

A peer can be stopped temporarily with the following command:

    weave stop

Such a peer will remember IP address allocation information on the
next `weave launch` but will forget any discovered peers or
modifications to the initial peer list that were made with `weave
connect` or `weave forget`. Note that if the host reboots, Docker
will restart the peer automatically.

### Remove a Peer

On peer to be removed:

    weave reset

Then optionally on each remaining peer:

    weave forget <removed peer>

This step is not mandatory, but it will eliminate log noise and
spurious network traffic by stopping reconnection attempts and
preventing further connection attempts after restart.
