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

[Fast datapath](/site/using-weave/fastdp.md) does not support
encryption. If you supply a password at `weave launch`, Weave Net
falls back to the slower `sleeve` mode for encrypted communication.

As an optimization, you can selectively disable data plane encryption
if some of your peers are co-located in a trusted network, for example
within the boundary of your own data center. List these networks using
the `--trusted-subnets` argument with `weave launch`:

    weave launch --password wfvAwt7sj --trusted-subnets 10.0.2.0/24,192.168.48.0/24

If *both* peers at the end of a connection consider the other to be in
a trusted subnet, Weave Net attempts to establish fast datapath
connectivity, which is unencrypted. Otherwise the slower `sleeve` mode
is used and communication is encrypted.

Configured trusted subnets are shown in [`weave status`](/site/troubleshooting.md#weave-status).

Be aware that:

 * Containers will be able to access the router REST API if fast datapath is disabled. You can prevent this by setting:
 [`icc=false`](https://docs.docker.com/engine/userguide/networking/default_network/container-communication/#communication-between-containers).
 * Containers are able to access the router control and data plane
  ports, but this can be mitigated by enabling encryption.

**See Also**

 * [Weave Encryption](/site/how-it-works/encryption.md)
 * [Using Fast Datapath](/site/using-weave/fastdp.md)
