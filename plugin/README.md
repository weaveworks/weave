# Weave network driver extension for Docker

A [plugin](http://docs.docker.com/engine/extend/plugin_api/) to
integrate [weave Net](http://weave.works/net/) with Docker.

## Setup:

Before you can use this plugin, you must [install weave](https://github.com/weaveworks/weave#installation) and [configure docker with a "cluster store"](https://docs.docker.com/engine/userguide/networking/get-started-overlay/#step-1-set-up-a-key-value-store).

    $ weave launch
    $ weave launch-plugin
    $ docker network create --driver=weave weave

When you start subsequent weave peers you have to tell them about other peers, e.g.

    $ weave launch <host1> <host2>
    $ weave launch-plugin

## Starting a container:

    $ docker run --net=weave -ti ubuntu

## Plugin command-line arguments

 * `--socket=<path>`, which is the socket on which to listen for the
   [plugin protocol](http://docs.docker.com/engine/extend/plugin_api/). 
   This defaults to what Docker expects, that is
   `"/run/docker/plugins/weave.sock"`, but you may wish to
   change it, for instance if you're running more than one instance of
   the driver for some reason.

 * `--log-level=debug|info|warning|error`, which tells the plugin
   how much information to emit for debugging.

## Points to note

Network plugins require Docker version 1.9 or later.

### Docker needs a "cluster store"

Weave operates as a "globally scoped" libnetwork driver, which means
libnetwork will assume all networks and endpoints are commonly known
to all hosts. It does this by using a shared database which you must
provide.

As a consequence, you need to supply Docker with the address of a "cluster
store" when you start it; for example, an etcd installation.

There's no specific documentation for using a cluster store, but the
first part of [this guide](https://github.com/docker/docker/blob/master/docs/userguide/networking/get-started-overlay.md) may help.

### WeaveDNS not supported

Weave's built-in service discovery mechanism is not currently
supported by the plugin.  However, Docker provides a rudimentary
discovery mechanism by writing all user-provided container names and
hostnames into every container's `/etc/hosts`.

### Restarting

We start the plugin with a restart policy of 'always', because
Docker attempts to recreate networks on startup which means the plugin
container has to be started. If it isn't started automatically by Docker
with a restart policy, it's not possible to start it manually because
Docker is blocked.
