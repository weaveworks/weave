---
title: Building Weave
layout: default
---

**NB** This is only necessary if you want to work on the weave code.

## Building directly on Linux

You can work on weave without using a VM if you are running the docker
daemon outside a VM. (These instructions have only been tested on
Ubuntu.)

To build weave you need `libpcap-dev` and `docker` installed. And `go`
(and `git` and `hg` to fetch dependencies).

The package name is `github.com/zettio/weave`, so assuming `$GOPATH`
is set:

```bash
$ cd $GOPATH
$ WEAVE=github.com/zettio/weave
$ git clone https://$WEAVE src/$WEAVE
$ cd src/$WEAVE
```

Then simply run

```bash
$ make -C weaver
```

This will build the weave router, produce a docker image
`zettio/weave` and export that image to `/tmp/weave.tar`.

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
vm$ make -C weaver
```

The docker daemon is also running in this VM, so you can then do

```bash
vm$ sudo weaver/weave launch 10.0.0.1/16
vm$ docker ps
```

and so on.

You can provide extra Vagrant configuration by putting a file
`Vagrant.local` in the same place as `Vagrantfile`; for instance, to
forward additional ports.
