# Concepts
## Installing Weave

    sudo curl -L git.io/weave -o /usr/local/bin/weave
    sudo chmod a+x /usr/local/bin/weave

## Force Download of Docker Images

    weave setup

## Launching Weave

    weave launch

## Starting Weave on Boot

    sudo weave install-unit

## Peer Identity

    weave launch --name

## Bootstrap Consensus

    weave consense

## Electorate

    weave launch --not-electorate

## Discovery & Connecting Peers

    weave launch [--no-discovery] [<peer> ...]
    weave connect [<peer> ...]
    weave forget [<peer> ...]

## Persistence
* `weave launch` command line peers
* IPAM ring (e.g. which peername owns which ranges of address space)
* IPAM allocations (e.g. which container on a host has which specific
  address)
* Additional DNS records for containers and weave:external
* Non-IPAM `weave attach` and `weave expose` addresses (implies
  persistence of `weave attach` and `weave expose`)

# Deployment Types
## Manual/Incremental
* Recommended for evaluation only
* Configuration built up incrementally via `weave` CLI
* Results of all CLI interactions are persisted
* Survives reboots if systemd unit file installed
* Unsophisticated users will probably try to use this for production,
  so should probably recommend that they start all nodes except the
  first with --not-elector (this is conceptually much easier than
  --init-peer-count IMO). It means that the first node is a SPOF
  initially, if that matters then choose a different use case

## Uniform cluster
* Uniform node configuration via systemd unit and environment file
* Minimum one initial node. Wait for consensus (ideally before initial
  commissioning is considered successful) before further changes occur
* All nodes are electors
* Supports grow via controlled process:
    * Update startup target list of existing peers (in case they
      reboot)
    * Launch new node listing all existing peers as targets
    * Wait for consensus on new peer
* Support shrink via controlled process:
    * `weave reset` on peer to be removed
    * Update startup target list of remaining peers (in case they
      reboot)
    * `weave forget` on remaining peers
* Grow/shrink operations must be serialised by some external
  management agent (e.g. a human operator or some automated
  infrastructure) to prevent cliques forming under partition

## Non-uniform cluster (fixed base + dynamic contingent)
* Builds on uniformly configured fixed cluster core (minimum one node,
  recommend two for resilience) of protected instances
* Supports automated add and remove of nodes by external management
  infrastructure in response to scale in/out events after consensus is
  reached in core
* Dynamic nodes are not electors, and are only configured to connect
  to fixed cluster. Sideways peering between dynamic nodes is obtained
  via gossip discovery
* Can launch or remove as many dynamic nodes as desired concurrently
* Dynamic removal must `weave reset` prior to instance termination
* Requires periodic checking for lost address space in case instances
  have been removed under partition

## Autoscaling From Zero Nodes And Back

(Editor's note: this is an attempt to get the 'can be automated to
scale up from zero nodes' feature of a uniform cluster but without the
serialisation requirement on further changes. I don't think it's
really viable)

* Requires management infrastructure to start initial node with
  different config (e.g. as an elector) and subsequent nodes as
  non-electors
* Will fail in a way which is awkward to recover automatically if the
  initial node dies as a second node is being launched
* Target peer specification is hard, generally involving touching the
  config of all existing nodes every time a node is added or removed
    * At a minimum each additional peer should list all existing peers
      as targets. Should update existing peers too for maximum
      resilience in the face of random node removal
    * Removing nodes is harder; need to adjust target lists of all
      remaining peers to avoid connection attempts to decommissioned
      nodes

# Maintenance Tasks
## Rolling Upgrades
## Forcing Consensus during Cluster Commissioning/Change

    weave consense

## Checking for and Recovering Lost Space

Need a way of determining deceased peers, e.g. in the case where a
scale-in event has happened under partition. As an operator I would
want to be able to generate a report on absent peers so that I can
configure a threshold alert to trigger manual intervention:

    weave rmpeer

# Additional features
* `weave consense`
* `sudo weave install-unit`
* /etc/weave.conf
* Derive peer name from hostname
* A way of finding dead address space (does `weave status ipam` do
  this?)

# Questions/Discussion points
* How does `weave rmpeer` behave during a partition? Does it complete
  immediately, and do the Right Thing once the partition heals?
* How long does `weave reset` wait during partition?
* --not-elector preferable to --electorate, given the above set of use
  cases
* I would prefer a weave.conf over having weave persist the results of
  certain weave CLI interactions in automated deployment scenarios:
    * weave attach
    * weave expose
    * weave dns-add

# Persistence Behaviour


