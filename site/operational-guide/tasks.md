---
title: Administrative Tasks
menu_order: 60
---

The following administrative tasks are discussed: 

* [Configuring Weave to Start Automatically on Boot](#start-on-boot)
* [Detecting and Reclaiming Lost IP Address Space](#detect-reclaim-ipam)
* [Upgrading a Cluster](#cluster-upgrade)
* [Resetting Persisted Data](#reset)


##<a name="start-on-boot"></a>Configuring Weave to Start Automatically on Boot

`weave launch` runs all of Weave's containers with a Docker restart
policy set to `always`.  If you have launched Weave manually at least
once and your system is configured to start Docker on boot, then Weave
will start automatically on system restarts.

If you are aiming for a non-interactive installation, use
systemd to launch Weave after Docker - see [systemd
docs](/site/installing-weave/systemd.md) for details.

##<a name="detect-reclaim-ipam"></a>Detecting and Reclaiming Lost IP Address Space

The recommended method of removing a peer is to run `weave reset` on that
peer before the underlying host is decommissioned or repurposed. This
ensures that the portion of the IPAM allocation range assigned to the
peer is released for reuse. 

Under certain circumstances this operation may not be successful, 
or possible:

* If the peer is partitioned from the rest of the network
  when `weave reset` is executed on it
* If the underlying host is no longer available to execute `weave
  reset` due to a hardware failure or other unplanned termination (for
  example when using autoscaling with spot-instances that can be
  destroyed without notice)

In some cases you may already be aware of the problem, as you were
unable to execute `weave reset` successfully or because you know
through other channels that the host has died. In these cases you can
proceed straight to the reclaiming lost address space section.

However in some scenarios it may not be obvious that space has been
lost, in which case you can check for it periodically with the
following command on any peer:

    weave status ipam

This command lists the names of unreachable peers. If you are satisifed
that they are truly gone, rather than temporarily unreachable due to a
partition, you can reclaim their space manually.

When a peer dies unexpectedly the remaining peers will consider its
address space to be unavailable even after it has remained unreachable
for prolonged periods. There is no universally applicable time limit
after which one of the remaining peers could decide unilaterally that
it is safe to appropriate the space for itself, and so an
administrative action is required to reclaim it.

The `weave rmpeer` command is provided to perform this task, and must
be executed on _one_ of the remaining peers. That peer will take
ownership of the freed address space.

##<a name="cluster-upgrade"></a>Upgrading a Cluster

Protocol versioning and feature negotiation are employed in Weave Net
to enable incremental rolling upgrades. Each major maintains
the ability to speak to the preceding major release at a minimum, and
connected peers only utilize features which both support. 

The general upgrade procedure is as follows:

On each peer:

* Stop the old Weave with `weave stop` (or `systemctl stop weave` if
  you're using a systemd unit file)
* Download the new Weave script and replace the existing one
* Start the new Weave with `weave launch <existing peer list>` (or
  `systemctl start weave` if you're using a systemd unit file)

Since the first launch with the new script pulls the new container images, this 
results in some downtime. To minimize downtime, download the new script 
to a temporary location first:

* Download the new Weave script to a temporary location, for example,
  `/path/to/new/weave`
* Pull the new images with `/path/to/new/weave setup`
* Stop the old Weave with `weave stop` (or `systemctl stop weave` if
  you're using a systemd unit file)
* Replace the existing script with the new one
* Start the new Weave with `weave launch <existing peer list>` (or
  `systemctl start weave` if you're using a systemd unit file)

>>**Note:** Always check the Release Notes for specific versions in case
there are any special caveats or deviations from the standard
procedure.

##<a name="reset"></a>Resetting Persisted Data

Weave Net persists information in a data volume container named
`weavedb`. If you wish to start from a completely clean slate (for
example to withdraw a peer from one network and join it to another)
you can issue the following command:

    weave reset

