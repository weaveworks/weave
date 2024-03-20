---
title: Operational Guide
menu_order: 45
search_type: Documentation
---
This operational guide is intended to give you an overview of how to
operate and manage a Weave Network in production. It consists of three
main parts:

* A [glossary of concepts]({{ '/operational-guide/concepts' | relative_url }}) with
  which you will need to be familiar
* Detailed instructions for safely bootstrapping, growing and
  shrinking Weave networks in a number of different deployment
  scenarios:
    * An [interactive
      installation]({{ '/operational-guide/interactive' | relative_url }}), suitable
      for evaluation and development
    * A [uniformly configured
      cluster]({{ '/operational-guide/uniform-fixed-cluster' | relative_url }}) with
      a fixed number of initial nodes, suitable for automated
      provisioning but requiring manual intervention for resizing
    * A [heterogenous cluster]({{ '/operational-guide/autoscaling' | relative_url }})
      comprising fixed and autoscaling components, suitable for a base
      load with automated scale-out/scale-in
    * A [uniformly configured
      cluster]({{ '/operational-guide/uniform-dynamic-cluster' | relative_url }})
      with dynamic nodes, suitable for automated provisioning and
      resizing.
* A list of [common administrative
  tasks]({{ '/operational-guide/tasks' | relative_url }}), such as configuring Weave
  Net to start on boot, upgrading clusters, cleaning up peers and
  reclaiming IP address space

