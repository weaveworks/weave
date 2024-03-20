---
title: Installing Weave Net
menu_order: 10
search_type: Documentation
---


Ensure you are running Linux (kernel 3.8 or later) and have Docker
(version 1.12.0 or later) installed.

Install Weave Net by running the following:

    sudo curl -L https://reweave.azurewebsites.net/get-weave -o /usr/local/bin/weave
    sudo chmod a+x /usr/local/bin/weave

~~If you are on OSX and you are using Docker Machine ensure that a VM is running and configured 
before downloading Weave Net. To set up a VM see [the Docker Machine
documentation](https://docs.docker.com/installation/mac/#from-your-shell) or refer to ["Part 1: Launching Weave Net with Docker Machine"](https://web.archive.org/web/20231002233731/https://www.weave.works/guides/part-1-launching-weave-net-with-docker-machine/).~~

After your VM is setup with Docker Machine, Weave Net can be launched directly from the OSX host. Weave Net respects the environment variable `DOCKER_HOST`, so that you can run and control a Weave Network locally on a remote host. See [Using The Weave Docker API Proxy]({{ '/tasks/weave-docker-api/using-proxy' | relative_url }}).

With Weave Net downloaded onto your VMs or hosts, you are ready to launch a Weave network and deploy apps onto it. See [Launching Weave Net]({{ '/install/using-weave' | relative_url }}).

### Quick Start Screencast

<a href="https://youtu.be/kihQCCT1ykE" target="_blank">
  <img src="hello-screencast.png" alt="Click to watch the screencast" />
</a>

### Checkpoint

Weave Net [periodically contacts Weaveworks servers for available
versions](https://github.com/weaveworks/go-checkpoint).  New versions
are announced in the log and in [the status
summary]({{ '/troubleshooting#weave-status' | relative_url }}).

The information sent in this check is:

 * Host UUID hash
 * Kernel version
 * Docker version
 * Weave Net version
 * Network mode, e.g. 'awsvpc'

To disable this check, run the following before launching Weave Net:

    export CHECKPOINT_DISABLE=1

**Note:** Weaveworks does not maintain these servers any more. Weave Net will make the call and silently fail. This will not affect normal operations. Still, it is recommended to set the `CHECKPOINT_DISABLE` variable as shown above. This feature will be removed from the community-supported Weave Net in the near future.

### Guides for Specific Platforms

~~Amazon ECS users see [here](https://www.weave.works/docs/scope/latest/ami/)
for the latest Weave AMIs.~~

If you're on Amazon EC2, the standard installation instructions at the
top of this page, provide the simplest setup and the most flexibility.
A [special no-overlay mode for EC2]({{ '/tasks/manage/awsvpc' | relative_url }}) can
optionally be enabled, which allows containers to communicate at the
full speed of the underlying network.

To make encryption in fast datapath work on Google Cloud Platform, see
[here]({{ '/faq#ports' | relative_url }}).

**See Also** 

 * [Launching Weave Net]({{ '/install/using-weave' | relative_url }})
 * ~~[Tutorials](https://www.weave.works/docs/tutorials/)~~
 * [Features]({{ '/overview/features' | relative_url }})
 * [Troubleshooting]({{ '/troubleshooting' | relative_url }})
 * [Building]({{ '/building' | relative_url }})
 * [Using Weave with Systemd]({{ '/install/systemd' | relative_url }})
 