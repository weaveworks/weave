---
title: Securing Connections Across Untrusted Networks
layout: default
---


To connect containers across untrusted networks, Weave peers can be instructed to encrypt traffic by supplying a `--password` option or by using the `WEAVE_PASSWORD` environment variable during `weave launch`. 

For example:

    host1$ weave launch --password wfvAwt7sj

or

    host1$ export WEAVE_PASSWORD=wfvAwt7sj
    host1$ weave launch

>NOTE: The command line option takes precedence over the environment variable._

> To avoid leaking your password via the kernel process table or your
> shell history, we recommend you store it in a file and capture it
> in a shell variable prior to launching weave: `export
> WEAVE_PASSWORD=$(cat /path/to/password-file)`

To guard against dictionary attacks, the password needs to be reasonably strong with at least 50 bits of entropy is recommended. An easy way to generate a random password that satisfies this requirement is:

    < /dev/urandom tr -dc A-Za-z0-9 | head -c9 ; echo

The same password must be specified for all Weave peers, by default both control and data plane traffic will then use authenticated encryption. 

Fast datapath does not support encryption. If you supply a
password at `weave launch` the router falls back to a slower
`sleeve` mode that does support encryption.

If some of your peers are co-located in a trusted network (for example within the boundary of your own datacenter) you can use the `--trusted-subnets` argument to `weave launch` to selectively disable data plane encryption as an optimization. 

Both peers must consider the other to be in a trusted subnet for this to take place - if they do not agree, Weave [falls back to a slower method]( /site/fastdp/using-fastdp.md) for transporting data between peers, since fast datapath does not support encryption.

Be aware that:

 * Containers will be able to access the router REST API if fast datapath is disabled. You can prevent this by setting:
 [`icc=false`](https://docs.docker.com/engine/userguide/networking/default_network/container-communication/#communication-between-containers)
 * Containers are able to access the router control and data plane
  ports, but this can be mitigated by enabling encryption.

**See Also**

 * [Using Encryption With Weave](/site/encryption/crypto-overview.md)
