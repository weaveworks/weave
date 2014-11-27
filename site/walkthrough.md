---
title: weave Walkthrough
---

## launch DNS by default, IP allocation and naming, both with sensible defaults, rendezvous peering, auto-attach

````bash
root@a:~# weave launch
root@a:~# weave run --name=pong -d -ti ubuntu
````
Weave automatically picked an IP range that isn't used locally (this range can also be specified explicitly, as an argument to `weave launch`), and allocated the container an IP in that range. The container is registered in weaveDNS as `pong.weave.local`. weaveDNS was launched automatically since no `--no-dns` option was given on `weave launch`.

````bash
root@b:~# weave launch
root@b:~# weave run -ti ubuntu
root@c0c8b4a7d709:/# ping pong
ping pong.weave.local (10.0.1.1) 56(84) bytes of data.
64 bytes from pong.weave.local (10.0.1.1): icmp_seq=1 ttl=64 time=0.036 ms
64 bytes from pong.weave.local (10.0.1.1): icmp_seq=2 ttl=64 time=0.031 ms
^C
--- pong.weave.local ping statistics ---
2 packets transmitted, 2 received, 0% packet loss, time 999ms
rtt min/avg/max/mdev = 0.031/0.033/0.036/0.006 ms
````
Weave peered with A through rendezvous (since both peers happened to be running in some local network), learned from A what the IP range is, checked that it is unused locally (it would complain if it was), allocated an IP from that range for the container, configured name resolution to go to weaveDNS and search in the weave.local domain. Note that the container is not registered in DNS since it wasn't explicitly named.

We didn't specify `-d` in `weave run` and therefore, just like in `docker run`, automatically attached to the container.

## load balancing

````bash
root@c:~# weave launch
root@c:~# weave run --name=pong -d -ti ubuntu
````

````bash
root@c0c8b4a7d709:/# ping pong
ping pong.weave.local (10.0.1.1) 56(84) bytes of data.
64 bytes from pong.weave.local (10.0.1.1): icmp_seq=1 ttl=64 time=0.036 ms
64 bytes from pong.weave.local (10.0.1.1): icmp_seq=2 ttl=64 time=0.031 ms
^C
--- pong.weave.local ping statistics ---
2 packets transmitted, 2 received, 0% packet loss, time 999ms
rtt min/avg/max/mdev = 0.031/0.033/0.036/0.006 ms
root@c0c8b4a7d709:/# ping pong
ping pong.weave.local (10.0.1.3) 56(84) bytes of data.
64 bytes from pong.weave.local (10.0.1.3): icmp_seq=1 ttl=64 time=0.036 ms
64 bytes from pong.weave.local (10.0.1.3): icmp_seq=2 ttl=64 time=0.031 ms
^C
--- pong.weave.local ping statistics ---
2 packets transmitted, 2 received, 0% packet loss, time 999ms
rtt min/avg/max/mdev = 0.031/0.033/0.036/0.006 ms
````

The container we started on `c` has the same name as the container we started on `a`. As a result, name resolution will return the addresses of both containers in random order.

(It may be more compelling to use `nc` instead of `ping` here, i.e. showing data arriving in one container and then the other. Also, I cannot think of a neat way of demoing that *all* records are returned in random order. Running `dig +noall +answer pong` a few times is the best I've come up with so far.)

## automatic, implicit subnet allocation

````bash
root@a:~# weave run --name=pong.foo -d -ti ubuntu
root@a:~# weave run --name=pong.bar -d -ti ubuntu
root@a:~# weave run --name=.foo -ti ubuntu
````
TODO: flesh this out

Name suffixes identify applications and thus subnets. Subnets are automatically allocated from a different free IP range than the default range (the range can also be specified as an extra argument to `weave launch`). All subnets are /24, though this too can be changed with a `weave launch` option. There's also a way (TBD) to have subnets of different size. Btw, some interesting scenarios arising here when different weave peers are launched with different options.

The magic `.` prefix indicates that the container should be anonymous, i.e. be given a random name, not registered in DNS but still be part of the given application and hence being able to resolve unqualified names in it.
