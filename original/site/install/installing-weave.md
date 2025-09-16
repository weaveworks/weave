---
title: Installing Weave Net
menu_order: 10
search_type: Documentation
---


Ensure you are running Linux (kernel 3.8 or later) and have Docker
(version 1.10.0 or later) installed.

Install Weave Net by running the following:

    sudo curl -L git.io/weave -o /usr/local/bin/weave
    sudo chmod a+x /usr/local/bin/weave

If you are on OSX and you are using Docker Machine ensure that a VM is running and configured 
before downloading Weave Net. To set up a VM see [the Docker Machine
documentation](https://docs.docker.com/installation/mac/#from-your-shell) or refer to ["Part 1: Launching Weave Net with Docker Machine"](/guides/part-1-launching-weave-net-with-docker-machine/).

After your VM is setup with Docker Machine, Weave Net can be launched directly from the OSX host. Weave Net respects the environment variable `DOCKER_HOST`, so that you can run and control a Weave Network locally on a remote host. See [Using The Weave Docker API Proxy](/site/tasks/weave-docker-api/using-proxy.md).

With Weave Net downloaded onto your VMs or hosts, you are ready to launch a Weave network and deploy apps onto it. See [Launching Weave Net](/site/install/using-weave.md).

### Quick Start Screencast

<a href="https://youtu.be/kihQCCT1ykE" target="_blank">
  <img src="hello-screencast.png" alt="Click to watch the screencast" />
</a>

### Checkpoint

Weave Net [periodically contacts Weaveworks servers for available
versions](https://github.com/weaveworks/go-checkpoint).  New versions
are announced in the log and in [the status
summary](/site/troubleshooting.md/#weave-status).

The information sent in this check is:

 * Host UUID hash
 * Kernel version
 * Docker version
 * Weave Net version
 * Network mode, e.g. 'awsvpc'

To disable this check, run the following before launching Weave Net:

    export CHECKPOINT_DISABLE=1

### Guides for Specific Platforms

Amazon ECS users see [here](https://www.weave.works/docs/scope/latest/ami/)
for the latest Weave AMIs.

If you're on Amazon EC2, the standard installation instructions at the
top of this page, provide the simplest setup and the most flexibility.
A [special no-overlay mode for EC2](/site/tasks/manage/awsvpc.md) can
optionally be enabled, which allows containers to communicate at the
full speed of the underlying network.

To make encryption in fast datapath work on Google Cloud Platform, see
[here](/site/faq.md/#ports).

**See Also** 

 * [Launching Weave Net](/site/install/using-weave.md)
 * [Tutorials](https://www.weave.works/docs/tutorials/)
 * [Features](/site/overview/features.md)
 * [Troubleshooting](/site/troubleshooting.md)
 * [Building](/site/building.md)
 * [Using Weave with Systemd](/site/install/systemd.md)
 
