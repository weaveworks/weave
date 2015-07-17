---
title: Installing Weave with Boot2Docker
layout: default
---

# Installing Weave with Boot2Docker

If you are running Docker inside the Boot2Docker VM, e.g. because you
are on a Mac, then the following changes are required to the weave
instructions. First, we need to get Boot2Docker running:

    host1$ boot2docker init
    host1$ boot2docker start
    host1$ eval "$(boot2docker shellinit)"

Assuming you have fetched the 'weave' script via curl or similar, and
it is in the current directory, launch weave on the Boot2Docker VM and
configure our shell. Because Boot2Docker uses TLS, we have to pass a
few extra options when launching the docker API proxy:

    host1$ ./weave launch-router
    host1$ ./weave launch-proxy --tls \
             --tlscacert /var/lib/boot2docker/tls/ca.pem \
             --tlscert /var/lib/boot2docker/tls/server.pem \
             --tlskey /var/lib/boot2docker/tls/serverkey.pem
    host1$ eval "$(./weave env)"

Now, we can use the docker command to run our containers, as shown in
the [proxy documentation](proxy.html):

    host1$ docker run -it ubuntu

For more information about how to access services running in
Boot2Docker from the host or other machines (i.e. outside of the weave
network), see these pages:

* https://github.com/docker/docker/issues/4007
* http://viget.com/extend/how-to-use-docker-on-os-x-the-missing-guide
