---
title: Troubleshooting Weave
layout: default
---

## Troubleshooting

Make sure you are running the latest version - you can download it
with

    docker pull zettio/weave

Check the weave container logs with

    docker logs weave

A reasonable amount of information, and all errors, get logged there.

The log verbosity can be increased by supplying the `-debug` flag when
launching weave. Be warned, this will log information on a per-packet
basis, so can produce a lot of output.

One can ask a weave router to report its status with

    weave status

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
