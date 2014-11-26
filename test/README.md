This directory contains integration tests for weave.

## Requirements

You need two VMs with docker >=1.3.1 installed and listening on TCP
port 2375 (see below). You also need to be able to ssh to these VMs,
preferably without having to input anything.

The `Vagrantfile` in this directory constructs two such VMs.

## Running tests

    ./setup.sh

uploads the weave images from where the Makefile puts them
(`/var/tmp`) to the two docker hosts, and copies the weave script
over.

Then you can use, e.g.,

    ./200_dns_test.sh

to run an individual test, or

    ./run_all.sh

to run everything named `*_test.sh`.

## Using other VMs

By default the tests assume the Vagrant VMs are used.

To use other VMs, set the environment variables <var>HOST1</var> and
<var>HOST2</var> to the IP addresses of two docker hosts, and set the
environment variable <var>SSH</var> to a command that will log into
either (which may just be `ssh`).

## Making docker available over TCP

To make docker listen to a TCP socket, you will usually need to either
run it manually with an option like `-H tcp://0.0.0.0:2375`; or, for
apt-get installed docker (Ubuntu and Debian), add the line

```
DOCKER_OPTS="--host unix:///var/run/docker.sock --host tcp://0.0.0.0:2375"
```

to the file `/etc/default/docker`, then restart docker.
