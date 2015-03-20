---
title: Building Weave
layout: default
---

You only need to build weave if you want to work on the weave codebase
(or you just enjoy building software).

Apart from the `weave` shell script, weave is delivered as a set of
container images.  There is no distribution-specific packaging, so in
principle it shouldn't matter which Linux distribution you build
under.  But naturally, Docker is a prerequisite (version 1.3.0 or
later).  And it is difficult to build under Fedora because [Fedora
does not include static
libraries](http://fedoraproject.org/wiki/Packaging:Guidelines#Packaging_Static_Libraries).
So we recommend building under Ubuntu.

You can also build in a container under any system that supports
Docker.  And you can run Ubuntu in a VM and build there.  These
options are described below.

## Building directly on Ubuntu

The weave git repository should be cloned into
`$GOPATH/src/github.com/zettio/weave`, in accordance with [the go
workspace conventions](https://golang.org/doc/code.html#Workspaces):

```bash
$ WEAVE=github.com/zettio/weave
$ git clone https://$WEAVE $GOPATH/src/$WEAVE
$ cd $GOPATH/src/$WEAVE
```

Some prerequisites are needed to build weave.  First, install Docker
if you haven't already, by following the instructions [on the Docker
site](https://docs.docker.com/installation/ubuntulinux/).  Note that
we recommend using the Docker-maintained `lxc-docker` package, rather
than the `docker.io` package which contains a very old version.  Then
install the other prerequisites for building with:

```bash
$ sudo apt-get install build-essential git golang mercurial libpcap-dev
```

Then to actually build, simply do:

```bash
$ make
```

This will build the weave components and package them into three
Docker images (`zettio/weave`, `zettio/weavedns`, and
`zettio/weaveexec`).  These are then exported (as `weave.tar`,
`weavedns.tar` and `weaveexec.tar`).

## Building in a Docker container

As a preliminary step, we create a container image based on Ubuntu
that has all the prerequisites.  This avoids the need to download and
install them for each build.  In the `weave` directory, do:

```bash
$ sudo docker build -t zettio/weave-build build
```

Next we run a container based on that image. That container requires
access to the Docker daemon on the host, via
`/var/run/docker.sock`. If you are building under a Fedora or RHEL
Docker host (or another distribution that uses SELinux), and you have
SELinux set to enforcing mode, it will block attempts to access
`/var/run/docker.sock` inside the container.  See
[dpw/selinux-dockersock](https://github.com/dpw/selinux-dockersock)
for a way to work around this problem.

To perform a build, run:

```bash
$ sudo docker run -v /var/run/docker.sock:/var/run/docker.sock zettio/weave-build https://github.com/zettio/weave.git
```

This will clone the weave git repository, then do the build.

When the build completes, the resulting images are stored in docker on
the host, as when building directly under Ubuntu.

The container arguments are passed to `git clone`, so for example, you
can build from a forked repository and a specific branch with:

```bash
$ sudo docker run -v /var/run/docker.sock:/var/run/docker.sock zettio/weave-build -b <branch name> <repo URI>
```

Alternatively, you might want to build from a weave source tree
already present on the host.  You can do this by using the `-v` option
to bind the bind your go workspace containing the weave repository to
`/home/go` inside the container.  No container arguments should be
passed in this case:

```bash
$ sudo docker run -v /var/run/docker.sock:/var/run/docker.sock -v <host gopath>:/home/go zettio/weave-build
```

This will leave the intermediate build artifacts on the host, so that
you can modify the weave source code and rebuild quickly.

## Building using Vagrant

If you aren't running Linux, or otherwise don't want to run the docker
daemon outside a VM, you can use
[Vagrant](https://www.vagrantup.com/downloads.html) to run a
development environment. You'll probably need to install
[VirtualBox](https://www.virtualbox.org/wiki/Downloads) too, for
Vagrant to run VMs in.

First, check out the code:

```bash
$ git clone https://github.com/zettio/weave
$ cd weave
```

The `Vagrantfile` in the top directory constructs a VM that has

 * docker installed
 * go tools installed
 * weave dependencies installed
 * $GOPATH set to ~
 * the local working directory mapped as a synced folder into the
   right place in $GOPATH

Once you are in the working directory you can issue

```bash
$ vagrant up
```

and wait for a while (don't worry, the long download and package
installation is done just once). The working directory is sync'ed with
`~/src/github.com/zettio/weave` on the VM, so you can edit files and
use git and so on in the regular filesystem.

To build and run the code, you need to use the VM. To log in and build
the weave image, do

```bash
$ vagrant ssh
vm$ cd src/github.com/zettio/weave
vm$ make
```

The docker daemon is also running in this VM, so you can then do

```bash
vm$ sudo ./weave launch
vm$ sudo docker ps
```

and so on.

You can provide extra Vagrant configuration by putting a file
`Vagrant.local` in the same place as `Vagrantfile`; for instance, to
forward additional ports.
