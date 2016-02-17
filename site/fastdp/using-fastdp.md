---
title: Using Fast Datapath
layout: default
---


##Using Fast Datapath

The most important thing to know about fast datapath is that you don't need to configure anything before using this feature. If you are using Weave Net 1.2 or greater, fast datapath (`fastdp`) is automatically enabled.

When Weave Net can't use the fast data path between two hosts, it falls back to the slower packet forwarding approach. Selecting the fastest forwarding approach is automatic, and is determined on a connection-by-connection basis. For example, a Weave network spanning two data centers might use fast data path within the data centers, but not for the more constrained network link between them. 

See [How Fastdp Works](/site/fastdp-how-it-works.md) for a more indepth discussion of this feature. 

###Disabling Fast Datapath

You can disable fastdp by enabling the `WEAVE_NO_FASTDP` environment variable at `weave launch`:

~~~bash
$ WEAVE_NO_FASTDP=true weave launch
~~~

###Fast Data Path and Encryption

Encryption does not work with fast datapath. If you enable encryption using the `--password` option to launch weave (or you use the `WEAVE_PASSWORD` environment variable), fast data path will by default be disabled. 

When encryption is not in use there may be other conditions in which the fastdp will revert back to `sleeve mode`. Once these conditions pass, weave will revert back to using fastdp. To view which mode Weave is using, run `weave status connections`.