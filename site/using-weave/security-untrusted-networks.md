---
title: Securing Connections Across Untrusted Networks
menu_order: 80
---


To connect containers across untrusted networks, Weave Net peers can be instructed to encrypt traffic by supplying a `--password` option or by using the `WEAVE_PASSWORD` environment variable during `weave launch`. 

For example:

    host1$ weave launch --password wfvAwt7sj

or

    host1$ export WEAVE_PASSWORD=wfvAwt7sj
    host1$ weave launch

>**NOTE**: The command line option takes precedence over the environment variable._

> To avoid leaking your password via the kernel process table or your
> shell history, we recommend you store it in a file and capture it
> in a shell variable prior to launching weave: `export
> WEAVE_PASSWORD=$(cat /path/to/password-file)`

To guard against dictionary attacks, the password needs to be reasonably strong with at least 50 bits of entropy is recommended. An easy way to generate a random password that satisfies this requirement is:

    < /dev/urandom tr -dc A-Za-z0-9 | head -c9 ; echo

The same password must be specified for all Weave Net peers, by default both control and data plane traffic will then use authenticated encryption. 

Fast datapath does not support encryption. If you supply a
password at `weave launch` the router falls back to a slower
`sleeve` mode that does support encryption.

If some of your peers are co-located in a trusted network (for example
within the boundary of your own data center) you can selectively
disable data plane encryption as an optimization, by listing such
networks via the `--trusted-subnets` argument to `weave launch`:

    weave launch --password wfvAwt7sj --trusted-subnets 10.0.2.0/24,192.168.48.0/24

>**Note:** Both peers must consider the other to be in a trusted subnet for this to take place - if they do not agree, Weave Net [falls back to a slower method](/site/using-weave/fastdp.md) for transporting data between peers, since fast datapath does not support encryption.

The configured trusted subnets are shown in [`weave status`](/site/troubleshooting.md#weave-status).

Be aware that:

 * Containers will be able to access the router REST API if fast datapath is disabled. You can prevent this by setting:
 [`icc=false`](https://docs.docker.com/engine/userguide/networking/default_network/container-communication/#communication-between-containers).
 * Containers are able to access the router control and data plane
  ports, but this can be mitigated by enabling encryption.

**See Also**

 * [Weave Encryption](/site/how-it-works/encryption.md)
 * [Using Fast Datapath](/site/using-weave/fastdp.md)
