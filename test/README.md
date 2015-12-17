This directory contains integration tests for weave.

## Requirements

You need two VMs with docker >=1.6.0 installed and listening on TCP
port 2375 (see below). You also need to be able to ssh to these VMs,
preferably without having to input anything.

The `Vagrantfile` in this directory constructs two such VMs.

To create the VMs, open a shell and in this directory and type

    vagrant up

To meet the aforementioned ssh requirement you may want to

    cp ~/.vagrant.d/insecure_private_key .

## Running tests

If you are [building weave using Vagrant](http://docs.weave.works/weave/latest_release/building.html),
it is recommended to run the tests from the build VM and not the host.

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

To use other VMs, set the environment variable <var>HOSTS</var> to the
space-separated list of IP addresses of the docker hosts, and set the
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

## Updating the GCE test image

When a new version of Docker is released, you'll need to update the GCE test image.

To do this, pick a fresh ```TEMPLATE_NAME``` and update any commands in
```function make_template``` in gce.sh, then run:

```
./gce.sh make_template
```

For this you'll need the GCE credentials, which can be found in ```bin/setup-circleci-secrets```,
which you'll need to decrypt and run (its echos the secrets into know locations):

```
./bin/setup-circleci-secrets "$SECRET_PASSWORD"
```

If you don't know the password, ask tom@weave.works.
