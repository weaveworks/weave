---
title: Concepts
menu_order: 10
---
This section describes some essential concepts with which you will
need to be familiar before moving on to the example deployment
scenarios.

## Host

For the purposes of this documentation we consider a host to be an
installation of the Linux operating system which is running an
instance of the Docker Engine. It may be executing directly on bare
hardware or inside a virtual machine.

## Peer

A peer is a running instance of Weave Net, typically one per host.

## Peer Name

Peers in the weave network are identified by a 48-bit value formatted
like an ethernet MAC address e.g. `01:23:45:67:89:ab`. This 'peer
name' is used for various purposes:

* Routing of packets between containers on the overlay network
* Recording the origin peer of DNS entries
* Recording ownership of IP address ranges

Whilst it is desirable for the peer name to remain stable across
restarts, it is essential that it is unique - if two or more peers
share the same name chaos will ensue, including but not limited to
double allocation of addresses and inability to route packets on the
overlay network. Consequently when the router is launched on a host it
derives its peer name in order of preference:

* From the command line; user is responsible for uniqueness and
  stability
* From the BIOS product UUID, which is generally stable across
  restarts and unique across different physical hardware and certain
  cloned VMs
* From the hypervisor UUID, which is generally stable across restarts
  and unique across VMs which do not provide access to a BIOS product
  UUID
* From a random value, practically unique across different physical
  hardware and cloned VMs but not stable across restarts

The appropriate strategy for assigning peer names depends on the type
and method of your particular deployment and is discussed in more
detail below.

## Peer Discovery

Peer discovery is a mechanism which allows peers to learn about new
weave hosts from existing peers without being explicitly told. Peer
discovery is
[enabled by default](/site/using-weave/finding-adding-hosts-dynamically.md).

## Network Partition

A network partition is a transient condition whereby some arbitrary
subsets of peers are unable to communicate with each other for the
duration - perhaps because a network switch has failed, or a fibre
optic line severed. Weave is designed to allow peers and their
containers to make maximum safe progress under conditions of
partition, healing automatically once the partition is over.

## IP Address Manager (IPAM)

[IPAM](/site/ipam.md) is the subsystem responsible for dividing up a
large contiguous block of IP addresses (known as the IP allocation
range) amongst peers so that individual addresses may be uniquely
assigned to containers anywhere on the overlay network.

When a new network is formed an initial division of the IP allocation
range must be made. Two (mutually exclusive) mechanisms with different
tradeoffs are provided to perform this task: seeding and consensus.

### Seeding

Seeding requires each peer to be told the list of peer names amongst
which the address space is to be divided initially. There are some
constraints and consequences:

* Every peer added to the network _must_ receive the same seed list,
  for all time, or they will not be able to join together to form a
  single cohesive whole
* Because the 'product UUID' and 'random value' methods of peer name
  assignment are unpredictable, the end user must by necessity also
  specify peer names
* Even though every peer _must_ receive the same seed, that seed does
  _not_ have to include every peer in the network, nor does it have to
  be updated when new peers are added (in fact due to the first
  constraint above it may not be)


Example configurations are given in the section on deployment
scenarios:

* [Uniform Dynamic Cluster](/site/operational-guide/uniform-dynamic-cluster.md)

### Consensus

Alternatively, when a new network is formed for the first time peers
can be configured to co-ordinate amongst themselves to automatically
divide up the IP allocation range. This process is known as consensus
and requires each peer to be told the total number of expected peers
(the 'initial peer count') in order to prevent formation of disjoint
groups of peers which would, ultimately, result in duplicate IP
addresses.

Example configurations are given in the section on deployment
scenarios:

* [Interactive Deployment](/site/operational-guide/interactive.md)
* [Uniform Fixed Cluster](/site/operational-guide/uniform-fixed-cluster.md)

### Observers

Finally, an option is provided to start a peer as an _observer_. Such
peers do not require either a seed peer name list nor an initial peer
count; instead they rely on the existence of other peers in the
network which have been so configured. When an observer needs address
space, it will ask for it from one of the peers which partook of the
initial division, triggering consensus if necessary.

Example configurations are given in the section on deployment
scenarios:

* [Autoscaling](/site/operational-guide/autoscaling.md)

## Persistence

Certain information is remembered between launches of weave (for
example across reboots):

* The division of the IP allocation range amongst peers
* Allocation of addresses to containers

The persistence of this information is managed transparently in a
volume container but can be
[destroyed explicitly](/site/operational-guide/tasks.md#reset)
if necessary.
