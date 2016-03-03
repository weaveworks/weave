---
title: Installing Weave Net
layout: default
---


Ensure you are running Linux (kernel 3.8 or later) and have Docker
(version 1.6.0 or later) installed. 

Install Weave Net by running the following:

    sudo curl -L git.io/weave -o /usr/local/bin/weave
    sudo chmod a+x /usr/local/bin/weave

If you are on OSX and are using Docker Machine) you need to make sure
that a VM is running and configured before getting Weave Net. Setting up a VM is shown in [the Docker Machine
documentation](https://docs.docker.com/installation/mac/#from-your-shell).
After the VM is configured with Docker Machine, Weave can be launched directly from the OSX host.

Weave respects the environment variable `DOCKER_HOST`, so that you can run
and control a Weave Network locally on a remote host. See [Using The Weave Docker API Proxy](/site/weave-docker-api/using-proxy.md)

With Weave downloaded onto your VMs or hosts, you are ready to launch a Weave network and deploy apps onto it. See [Deploying Applications to Weave](/site/using-weave/deploying-applications.md#launching)

CoreOS users see [here](https://github.com/fintanr/weave-gs/blob/master/coreos-simple/user-data) for an example of installing Weave using cloud-config.

Amazon ECS users see
[here](https://github.com/weaveworks/guides/blob/master/aws-ecs/LATESTAMIs.md)
for the latest Weave AMIs and
[here](http://weave.works/guides/service-discovery-with-weave-aws-ecs.html) to get started with Weave on ECS.


###Quick Start Screencast

<a href="https://youtu.be/kihQCCT1ykE" alt="Click to watch the screencast" target="_blank">
  <img src="/docs/hello-screencast.png" />
</a>

**See Also** 

 * [Using Weave Net](/site/using-weave/intro-example.md)
 * [Getting Started Guides](http://www.weave.works/guides/)
 * [Features](/site/features.md)
 * [Troubleshooting](/site/troubleshooting.md)
 * [Building](/site/building.md)
 * [Using Weave with Systemd](/site/systemd.md)
 