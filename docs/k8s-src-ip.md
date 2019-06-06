# Source IP addr of packet sent to k8s Service

## Setup

* Weave Net: v2.2.1.
* Kubernetes: v1.10.0.
* kube-proxy: iptables/ipvs mode with `--masquerade-all=false` and `--cluster-cidr` unspecified(default kubeadm options).
* Hosts: `h1` and `h2`.

## Source IP

\# | Src         | Target            | Dst    | Src IP           | `-j WEAVE-NPC`
---|-------------|-------------------|--------|------------------|--------------
1  | Pod_h1      | ip(Pod_h2)        | Pod_h2 | ip(Pod_h1)       | OK
2  | h1          | ClusterIP         | Pod_h1 | ip(weave_h1)     | **NOK**
3  | h1          | ClusterIP         | Pod_h2 | ip(weave_h1)     | OK
4  | Pod_h1      | ClusterIP         | Pod_h1 | ip(Pod_h1)       | OK
5  | Pod_h1      | ClusterIP         | Pod_h2 | ip(weave_h1)     | OK
6  | h1          | ip(h1):NodePort   | Pod_h1 | ip(weave_h1)     | **NOK**
7  | h1          | ip(h1):NodePort   | Pod_h2 | ip(weave_h1)     | OK
8  | h1          | ip(h2):NodePort   | Pod_h1 | ip(weave_h2)     | OK
9  | h1          | ip(h2):NodePort   | Pod_h2 | ip(weave_h2)     | ??? Can't reproduce
10 | Pod_h1      | ip(h1):NodePort   | Pod_h1 | ip(weave_h1)     | **NOK**
11 | Pod_h1      | ip(h1):NodePort   | Pod_h2 | ip(weave_h1)     | OK
12 | Pod_h1      | ip(h2):NodePort   | Pod_h1 | ip(weave_h2)     | OK
13 | Pod_h1      | ip(h2):NodePort   | Pod_h2 | ip(weave_h2)     | OK

Remarks:

* **Src IP** is of a packet which is captured on the weave bridge.
* **-j WEAVE-NPC** - whether a packet enters the `filter/WEAVE-NPC` iptables chain (OK = NetworkPolicy is enforced as required).
* **Pod_h1** - a Pod running on the `h1` host.
* **ip(weave_h1)** - IP addr of the weave bridge on the `h1` host.
