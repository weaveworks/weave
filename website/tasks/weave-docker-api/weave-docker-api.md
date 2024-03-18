---
title: Integrating Docker via the API Proxy
menu_order: 5
search_type: Documentation
---

The Docker API proxy automatically attaches containers to the Weave
network when they are started using the ordinary Docker
[command-line interface](https://docs.docker.com/reference/commandline/cli/)
or the [remote API](https://docs.docker.com/reference/api/docker_remote_api/).

### <a name="attaching-containers"></a>Attaching Containers to a Weave Network

There are three ways to attach containers to a Weave network (which method to use is
entirely up to you):

**1.** The Weave Net Docker API Proxy. See [Setting Up the Weave Net Docker API Proxy](#weave-api-proxy).

**2.**  The Docker Network Plugin framework. The Docker Network Plugin is used when
Docker containers are started with the --net flag, for example:

`docker run --net <docker-run-options>`

**Where,**

 * `<docker-run-options>` are the [docker run options](https://docs.docker.com/engine/reference/run/)
 you give to your container on start

Note that if a Docker container is started with the --net flag, then the Weave Docker API Proxy
is automatically disabled and is not used to attach containers.
See [Integrating Docker via the Network Plugin (Legacy)]({{ '/install/plugin/plugin' | relative_url }}) and
[Integrating Docker via the Network Plugin (V2)]({{ '/install/plugin/plugin-v2' | relative_url }}).

**3.** Containers can also be attached to the Weave network with `weave attach` commands. This method also
does not use the Weave Docker API Proxy.
See [Dynamically Attaching and Detaching Containers]({{ '/tasks/manage/dynamically-attach-containers' | relative_url }}).

### <a name="weave-api-proxy"></a>Setting Up The Weave Net Docker API Proxy

The proxy sits between the Docker client (command line or API) and the
Docker daemon, and intercepts the communication between the two.
It is started along with the router and weaveDNS when you run:

    host1$ weave launch

**N.B.**: Prior to version 2.0, the `launch-proxy` command allowed to pass configuration options and to start the proxy independently.
This command has been removed in 2.0 and `launch` now also accepts configuration options for the proxy.


By default, the proxy decides where to listen based on how the
launching client connects to Docker. If the launching client connected
over a UNIX socket, the proxy listens on `/var/run/weave/weave.sock`. If
the launching client connects over TCP, the proxy listens on port
12375, on all network interfaces. This can be adjusted using the `-H`
argument, for example:

    host1$ weave launch -H tcp://127.0.0.1:9999

If no TLS or listening interfaces are set, TLS is auto-configured
based on the Docker daemon's settings, and the listening interfaces are
auto-configured based on your Docker client's settings.

Multiple `-H` arguments can be specified. If you are working with a
remote docker daemon, then any firewalls in between need to be
configured to permit access to the proxy port.

All docker commands can be run via the proxy, so it is safe to adjust
your `DOCKER_HOST` to point at the proxy. Weave Net provides a convenient
command for this:

    host1$ eval $(weave env)
    host1$ docker ps

The prior settings can be restored with

    host1$ eval $(weave env --restore)

Alternatively, the proxy host can be set on a per-command basis with

    host1$ docker $(weave config) ps

The proxy can be stopped, along with the router and weaveDNS, with

    host1$ weave stop

If you set your `DOCKER_HOST` to point at the proxy, you should revert
to the original settings prior to running `stop`.


**See Also**

 * [Using The Weave Docker API Proxy]({{ '/tasks/weave-docker-api/using-proxy' | relative_url }})
 * [Securing Docker Communications With TLS]({{ '/tasks/weave-docker-api/securing-proxy' | relative_url }})
 * [Launching Containers With Weave Run (without the Proxy)]({{ '/tasks/weave-docker-api/launching-without-proxy' | relative_url }})


