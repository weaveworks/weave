---
title: Launching Containers With Weave Run (without the Proxy)
menu_order: 50
---


If you don't want to use the proxy, you can also launch
containers on to the Weave network using `weave run`:

    $ weave run -ti weaveworks/ubuntu

The arguments after `run` are passed through to `docker run`. Therefore you
can freely specify whatever Docker options you need. 

Once the container is started, `weave run` attaches it to the Weave network, and in
this example, it obtains an automatically allocated IP. 

You can specify IP addresses manually instead:

    $ weave run 10.2.1.1/24 -ti weaveworks/ubuntu

`weave run` rewrites `/etc/hosts` in the same way
[the proxy does](/site/weave-docker-api/name-resolution-proxy.md). If you need to keep
the original file, specify `--no-rewrite-hosts` when running
the container:

    $ weave run --no-rewrite-hosts 10.2.1.1/24 -ti weaveworks/ubuntu

There are some limitations to starting containers using `weave run`:

* containers are always started in the background, i.e. the equivalent
  of always supplying the -d option to docker run
* the --rm option to docker run, for automatically removing containers
  after they stop, is not available
* the Weave network interface may not be available immediately on
  container startup.

Finally, there is a `weave start` command which starts existing
containers using `docker start` and attaches them to the Weave network.


**See Also**

 * [Setting Up The Weave Docker API Proxy](/site/weave-docker-api.md)
 * [Securing Docker Communications With TLS](/site/weave-docker-api/securing-proxy.md)
 * [Name Resolution via `/etc/hosts`](/site/weave-docker-api/name-resolution-proxy.md)
