---
title: Building Weave
layout: default
---

## Building

(NB. This is only necessary if you want to work on weave. Also, these
instructions have only been tested on Ubuntu.)

To build weave you need `libpcap-dev` and `docker` installed. And `go`
(and `git` and `hg` to fetch dependencies).

The package name is `github.com/zettio/weave`, so assuming `$GOPATH`
is set:

    $ cd $GOPATH
    $ WEAVE=github.com/zettio/weave
    $ git clone https://$WEAVE src/$WEAVE
    $ cd src/$WEAVE

Then simply run

    $ make -C weaver

This will build the weave router, produce a docker image
`zettio/weave` and export that image to /tmp/weave.tar
