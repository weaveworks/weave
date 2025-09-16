---
title: Integrating Kubernetes via the Addon
menu_order: 20
search_type: Documentation
---

The following topics are discussed:

* [Installation](#install)
   * [Installing on GKE](#gke)
   * [Installing on EKS](#eks)
   * [Upgrading the Daemon Sets](#daemon-sets)
   * [CPU and Memory Requirements](#resources)
   * [Pod Eviction](#eviction)
* [Features](#features)
   * [Pod Network](#pod-network)
   * [Network Policy](#npc)
* [Troubleshooting](#troubleshooting)
   * [Reading the logs](#logs)
   * [Troubleshooting Blocked Connections](#blocked-connections)
   * [Things to watch out for](#key-points)
   * [Troubleshooting FailedCreatePodSandBox errors](#failedcreatepodsandbox)
* [Changing Configuration Options](#configuration-options)
* [Securing The Setup](#securing-the-setup)


## <a name="install"></a> Installation

*Before installing Weave Net, you should make sure the following ports are not
blocked by your firewall: TCP 6783 and UDP 6783/6784.
For more details, see the [FAQ]({{ '/faq#ports' | relative_url }}).*

[Weave Net](https://github.com/rajch/weave/releases) can be installed onto your CNI-enabled Kubernetes cluster with a single command:

```
$ kubectl apply -f https://reweave.azurewebsites.net/k8s/v1.29/net.yaml
```

Replace the `v1.29` with the version of Kubernetes running on your cluster.

**Important:** this configuration won't enable encryption by default. If your data plane traffic isn't secured that could allow malicious actors to access your pod network. Read on to see the alternatives.

After a few seconds, a Weave Net pod should be running on each
Node and any further pods you create will be automatically attached to the Weave
network.

**Note:** This command requires Kubernetes 1.4 or later, and we
recommend your master node has at least two CPU cores.

> CNI, the [_Container Network Interface_](https://github.com/containernetworking/cni),
> is a proposed standard for configuring network interfaces for Linux
> containers.
>
> If you do not already have a CNI-enabled cluster, you can bootstrap
> one easily with
> [kubeadm](http://kubernetes.io/docs/getting-started-guides/kubeadm/).
>
> Alternatively, you can [configure CNI yourself](http://kubernetes.io/docs/admin/network-plugins/#cni)
>
> Weave net depends on the *portmap* [standard](https://github.com/containernetworking/plugins) CNI plugin
> to support *hostport* functionality. Please ensure that *portmap* CNI plugin is installed 
> (either by cluster installers like kubeadm or manually if you have configured CNI yourself) in `/opt/cni/bin` directory.


**Note:** If using the [Weave CNI
Plugin]({{ '/kubernetes' | relative_url }}) from a prior full install of Weave Net with your
cluster, you must first uninstall it before applying the Weave-kube addon.
Shut down Kubernetes, and _on all nodes_ perform the following:

 * `weave reset`
 * Remove any separate provisions you may have made to run Weave at
   boot-time, e.g. `systemd` units
 * `rm /opt/cni/bin/weave-*`

Then relaunch Kubernetes and install the addon as described
above.

### <a name="gke"></a> Installing on GKE
Please note that you must grant the user the ability to create roles in Kubernetes before launching Weave Net.
This is a prerequisite to use use role-based access control on GKE. Please see the GKE [instructions](https://cloud.google.com/kubernetes-engine/docs/how-to/role-based-access-control).

### <a name="gke"></a> Installing on EKS

EKS by default installs `amazon-vpc-cni-k8s` CNI. Please follow below steps to use Weave-net as CNI

- create EKS cluster in any of [prescribed](https://docs.aws.amazon.com/eks/latest/userguide/create-cluster.html) way
- delete `amazon-vpc-cni-k8s` daemonset by running `kubectl delete ds aws-node -n kube-system` command
- delete `/etc/cni/net.d/10-aws.conflist` on each of the node
- edit instance security group to allow TCP 6783 and UDP 6783/6784 ports
- flush iptables nat, mangle, filter tables to clear any iptables configurations done by `amazon-vpc-cni-k8s`
- restart kube-proxy pods to reconfigure iptables
- apply weave-net daemoset by following above installation steps
- delete existing pods so they get recreated in Weave pod CIDR's address-space. 

Please note that while pods can connect to the Kubernetes API server for your cluser, API server will not be able to connect to the pods as API server nodes are not connected to Weave Net (they run on network managed by EKS).


### <a name="daemon-sets"></a> Upgrading the Daemon Sets

The DaemonSet definition specifies [Rolling
Updates](https://kubernetes.io/docs/tasks/manage-daemon/update-daemon-set/),
so when you apply a new version Kubernetes will automatically restart
the Weave Net pods one by one.

## <a name="resources"></a>CPU and Memory Requirements

Kubernetes manages
[resources](https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/)
on each node, and only schedules pods to run on nodes that have enough
free resources.

In the example manifests we request 10% of a CPU, which will be enough
for small installations, but you should monitor how much it uses in
your production clusters and adjust the requests to suit.

We do not recommend that you set a CPU or memory _limit_ unless you
are very experienced in such matters, because the implementation of
limits in the Linux kernel sometimes behaves in a surprising way.

On a 1-node single-CPU cluster you may find Weave Net does not install
at all, because other Kubernetes components already take 95% of the
CPU. The best way to resolve this issue is to use machines with at
least two CPU cores.

## <a name="eviction"></a>Pod Eviction

If a node runs out of CPU, memory or disk, Kubernetes may [decide to
evict](https://kubernetes.io/docs/concepts/cluster-administration/out-of-resource/)
one or more pods. It may choose to evict the Weave Net pod, which will
disrupt pod network operations.

You can reduce the chance of eviction by changing the DaemonSet to
have a much bigger request, and a limit of the same value.

This causes Kubernetes to apply a ["guaranteed" rather than a
"burstable" policy](https://kubernetes.io/docs/concepts/workloads/pods/pod-qos/).
However a similar request for disk space can not
be made, and so please be aware of this issue and monitor your
resources to ensure that they stay below 100%.

You can see when pods have been evicted via the `kubectl get events` command

```
LASTSEEN   COUNT     NAME          KIND    TYPE      REASON     SOURCE            MESSAGE
1m         1         mypod-09vkd   Pod     Warning   Evicted    kubelet, node-1   The node was low on resource: memory.
```

or `kubectl get pods`

```
NAME                READY     STATUS    RESTARTS   AGE       IP          NODE
mypod-09vkd         0/1       Evicted   0          1h        <none>      node-1
```

If you see this in your cluster, consider some of the above steps to
reduce disruption.

## <a name="features"></a>Features

### <a name="pod-network"></a>Pod Network

Weave Net provides a network to connect all pods together,
implementing the [Kubernetes
model](https://kubernetes.io/docs/concepts/cluster-administration/networking/#kubernetes-model).

Kubernetes uses the _Container Network Interface_
([CNI](https://github.com/containernetworking/cni)) to join pods onto Weave Net.

Kubernetes implements many network features itself on top of the pod
network.  This includes
[Services](https://kubernetes.io/docs/concepts/services-networking/service/),
[Service Discovery via DNS](https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/)
and [Ingress into the cluster](https://kubernetes.io/docs/concepts/services-networking/ingress/).
WeaveDNS is disabled when using the Kubernetes addon.

### <a name="npc"></a>Network Policy

[Kubernetes Network Policies](https://kubernetes.io/docs/concepts/services-networking/network-policies/) let
you securely isolate pods from each other based on namespaces and
labels. For more information on configuring network policies in
Kubernetes see the
[walkthrough](https://kubernetes.io/docs/tasks/administer-cluster/declare-network-policy/)
and the [NetworkPolicy API object
definition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#networkpolicy-v1-networking-k8s-io)

**Note:** as of version 1.9 of Weave Net, the Network Policy
  Controller allows all multicast traffic. Since a single multicast
  address may be used by multiple pods, we cannot implement rules to
  isolate them individually.  You can turn this behaviour off (block
  all multicast traffic) by adding `--allow-mcast=false` as an
  argument to `weave-npc` in the YAML configuration.

**Note:** Since ingress traffic is masqueraded, it makes sense to use
`ipBlock` selector in an ingress rule only when limiting access to a Service
annotated with `externalTrafficPolicy=Local` or between Pods when `podIP` is used
to access a Pod.

## <a name="troubleshooting"></a> Troubleshooting

The first thing to check is whether Weave Net is up and
running. The `kubectl apply` command you used to install it only
_requests_ that it be downloaded and started; if anything goes wrong
at startup, those details will only be visible in the [logs](#logs) of
the container(s).

To check what is running:

```
$ kubectl get pods -n kube-system -l name=weave-net
NAME              READY  STATUS   RESTARTS  AGE
weave-net-1jkl6   2/2    Running  0         1d
weave-net-bskbv   2/2    Running  0         1d
weave-net-m4x1b   2/2    Running  0         1d
```

You should see one line for each node in your cluster; each line
should have STATUS "Running", and READY should be 2 out of 2. If you
see a STATUS like "Error" or "CrashLoopBackoff", look in the logs of
the container with that status.

### <a name="logs"></a> Reading the logs

Pick one of the pods from the list output by `kubectl get pods` and
ask for the logs like this:

```
$ kubectl logs -n kube-system weave-net-1jkl6 weave
```

For easier viewing, pipe the output into a file, especially if
it is long.

By default log level of `weave` container is set to `info` level. If you wish to see more detailed logs you can set the desired log level for the `--log-level` flag through the `EXTRA_ARGS` environment variable for the `weave` container in the weave-net daemon set. Add environment variable as below.

```yaml
      containers:
      - command:
        - /home/weave/launch.sh
        name: weave
        env:
        - name: EXTRA_ARGS
          value: --log-level=debug
```

You may also set the `--log-level` flag to `warning` or `error` if you
prefer to only log exceptional conditions.

Many Kubernetes network issues occur at a higher level than Weave Net.
The [Kubernetes Service Debugging Guide](https://kubernetes.io/docs/tasks/debug-application-cluster/debug-service/)
has a detailed step-by-step guide.

Once it is up and running, the status of Weave Net can be checked by
running its CLI commands. This can be done in various ways:

1\. [Install the `weave` script]({{ '/install/installing-weave' | relative_url }}) and run:

```
$ weave status
        Version: 2.0.1 (up to date; next check at 2017/07/10 13:49:29)

        Service: router
       Protocol: weave 1..2
           Name: 42:8e:e8:c4:52:1b(host-0)
     Encryption: disabled
  PeerDiscovery: enabled
        Targets: 3
    Connections: 3 (2 established, 1 failed)
          Peers: 3 (with 6 established connections)
 TrustedSubnets: none

        Service: ipam
         Status: ready
          Range: 10.32.0.0/12
  DefaultSubnet: 10.32.0.0/12
```

2\. If you don't want to install additional software onto your hosts, run via `kubectl` commands, which produce the exact same outcome as the previous example:

```
### Identify the Weave Net pods:

$ kubectl get pods -n kube-system -l name=weave-net -o wide
NAME              READY  STATUS   RESTARTS  AGE  IP           NODE
weave-net-1jkl6   2/2    Running  0         1d   10.128.0.4   host-0
weave-net-bskbv   2/2    Running  0         1d   10.128.0.5   host-1
weave-net-m4x1b   2/2    Running  0         1d   10.128.0.6   host-2
```

The above shows all Weave Net pods available in your cluster.
You can see Kubernetes has deployed one Weave Net pod per host, in order to interconnect all hosts.

You then need to:
 - choose which pod you want to run your command from (in most cases it doesn't matter
   which one you pick so just pick the first one, e.g. pod `weave-net-1jkl6` here)
 - use `kubectl exec` to run the `weave status` command
 - specify the absolute path `/home/weave/weave` and add `--local` because it's running inside a container

```
$ kubectl exec -n kube-system weave-net-1jkl6 -c weave -- /home/weave/weave --local status

        Version: 2.0.1 (up to date; next check at 2017/07/10 13:49:29)

        Service: router
       Protocol: weave 1..2
           Name: 42:8e:e8:c4:52:1b(host-0)
     Encryption: disabled
  PeerDiscovery: enabled
        Targets: 3
    Connections: 3 (2 established, 1 failed)
          Peers: 3 (with 6 established connections)
 TrustedSubnets: none

        Service: ipam
         Status: ready
          Range: 10.32.0.0/12
  DefaultSubnet: 10.32.0.0/12
```

### <a name="blocked-connections"></a> Troubleshooting Blocked Connections

If you suspect that legitimate traffic is being blocked by the Weave Network Policy Controller, the first thing to do is check the `weave-npc` container's logs.

To do this, first you have to find the name of the Weave Net pod running on the relevant host:

```
$ kubectl get pods -n kube-system -o wide | grep weave-net
weave-net-08y45                  2/2       Running   0          1m        10.128.0.2   host1
weave-net-2zuhy                  2/2       Running   0          1m        10.128.0.4   host3
weave-net-oai50                  2/2       Running   0          1m        10.128.0.3   host2
```

Select the relevant container, for example, if you want to look at host2 then pick `weave-net-oai50` and run:

```
$ kubectl logs <weave-pod-name-as-above> -n kube-system weave-npc
```

When the Weave Network Policy Controller blocks a connection, it logs the following details about it:

* protocol used, 
* source IP and port, 
* destination IP and port, 

as per the below example:

```
TCP connection from 10.32.0.7:56648 to 10.32.0.11:80 blocked by Weave NPC.
UDP connection from 10.32.0.7:56648 to 10.32.0.11:80 blocked by Weave NPC.
```

### <a name="key-points"></a> Things to watch out for

- ~~Weave Net does not work on hosts running iptables 1.8 or above, only with 1.6.
  Track this via issue [#3465](https://github.com/weaveworks/weave/issues/3465)~~
- Don't turn on `--masquerade-all` on kube-proxy: this will change the
  source address of every pod-to-pod conversation which will make it
  impossible to correctly enforce network policies that restrict which
  pods can talk.
- If you do set the `--cluster-cidr` option on kube-proxy, make sure
  it matches the `IPALLOC_RANGE` given to Weave Net (see below).
- IP forwarding must be enabled on each node, in order for pods to
  access Kubernetes services or other IP addresses on another
  network. Check this with `sysctl net.ipv4.ip_forward`; the result
  should be `1`. (Be aware that there can be security implications of
  enabling IP forwarding).
- Weave Net can be run on minikube v0.28 or later with the default CNI config shipped with minikube
  being disabled. See [#3124](https://github.com/weaveworks/weave/issues/3124#issuecomment-397820940)
  for more details.
- ~~Weave Net has a problem with containerd versions 1.6.0 through 1.6.4. See [Troubleshooting FailedCreatePodSandBox errors](#failedcreatepodsandbox) below.~~

The following documents a problem in Weave Net before version 2.8.2. The problem does not exist any longer. The documentation is being kept as is, for now.

>  ### <a name="failedcreatepodsandbox"></a> Troubleshooting FailedCreatePodSandBox errors
>
>  If your Kubernetes cluster uses the `containerd` runtime (versions 1.6.0 through 1.6.4), Weave Net will not be able to allocate IP addresses to pods. Your pods, except the ones that use HostNetworking, will be stuck at `ContainerCreating` status.
>
>  You can examine any pod so affected  by running `kubectl describe`, for example:
>
>  ```
>  $ kubectl describe pod -n kube-system coredns-78fcd69978-dbxs9
>  ```
>
>  The events section will show repeated errors like the following:
>
>  ```
>    Warning  FailedCreatePodSandBox  3m6s                  kubelet            Failed to create pod sandbox: rpc error: code = Unknown desc = failed to setup network for sandbox "09a23f79c96333b9f54e12df54e817837c8021cbaa32bdfeefbe2a1fb215d9ef": plugin type="weave-net" name="weave" failed (add): unable to allocate IP address: Post "http://127.0.0.1:6784/ip/09a23f79c96333b9f54e12df54e817837c8021cbaa32bdfeefbe2a1fb215d9ef": dial tcp 127.0.0.1:6784: connect: connection refused
>  ```
>
>  You can verify that you are running an affected version of containerd by using the following:
>
>  ```
>  $ kubectl get nodes -o wide
>
>  NAME     STATUS   ROLES                  AGE   VERSION    INTERNAL-IP      EXTERNAL-IP   OS-IMAGE                         KERNEL-VERSION    CONTAINER-RUNTIME
>  host-1   Ready    control-plane,master   13m   v1.22.10   172.21.107.129   <none>        Debian GNU/Linux 11 (bullseye)   5.10.0-14-amd64   containerd://1.6.4
>  ```
>
>  The last column shows the container runtime and version.
>
>  Alternatively, you can run:
>
>  ```
>  $ containerd -v
>  containerd containerd.io 1.6.4 212e8b6fa2f44b9c21b2798135fc6fb7c53efc16
>  ```
>
>  The problem can be solved by upgrading containerd to v1.6.5 or above. For example, on Debian Linux, using the docker official repositories, you can use:
>
>  ```
>  sudo apt install containerd.io=1.6.6-1
>  ```
>
>  The problem occurs because of a behaviour change in cni v1.1.0, which caused a regression issue in Weave. It was corrected in cni v1.1.1. Containerd 1.6.5 onwards uses cni 1.1.1 and above. 

## <a name="configuration-options"></a> Changing Configuration Options

#### Manually editing the YAML file

You can manually edit the YAML file downloaded from our [releases page](https://github.com/rajch/weave/releases), 

For example,
- additional arguments may be supplied to the Weave router process by adding them to the `command:` array in the YAML file,
- additional parameters can be set via the environment variables listed above; these can be inserted into the YAML file like this:

```
      containers:
        - name: weave
          env:
            - name: IPALLOC_RANGE
              value: 10.0.0.0/16
```

The list of variables you can set is:

* `CHECKPOINT_DISABLE` - if set to 1, disable checking for new Weave Net
  versions (default is blank, i.e. check is enabled)
* `CONN_LIMIT` - soft limit on the number of connections between
  peers. Defaults to 200.
* `HAIRPIN_MODE` - Weave Net defaults to enabling hairpin on the bridge side of
  the `veth` pair for containers attached. If you need to disable hairpin, e.g. your
  kernel is one of those that can panic if hairpin is enabled, then you can disable it
  by setting `HAIRPIN_MODE=false`.
* `IPALLOC_RANGE` - the range of IP addresses used by Weave Net
  and the subnet they are placed in (CIDR format; default `10.32.0.0/12`)
* `EXPECT_NPC` - set to 0 to disable Network Policy Controller (default is on)
* `KUBE_PEERS` - list of addresses of peers in the Kubernetes cluster
  (default is to fetch the list from the api-server)
* `IPALLOC_INIT` - set the initialization mode of the [IP Address
  Manager]({{ '/operational-guide/concepts#ip-address-manager' | relative_url }})
  (defaults to consensus amongst the `KUBE_PEERS`)
* `WEAVE_EXPOSE_IP` - set the IP address used as a gateway from the
  Weave network to the host network - this is useful if you are
  configuring the addon as a static pod.
* `WEAVE_METRICS_ADDR` - address and port that the Weave Net
  daemon will serve Prometheus-style metrics on (defaults to 0.0.0.0:6782)
* `WEAVE_PASSWORD` - shared key to use during session key generation to encrypt
traffic between peers.
* `WEAVE_STATUS_ADDR` - address and port that the Weave Net
  daemon will serve status requests on (defaults to disabled).
* `WEAVE_MTU` - Weave Net defaults to 1376 bytes, but you can set a
  smaller size if your underlying network has a tighter limit, or set
  a larger size for better performance if your network supports jumbo
  frames - see [here]({{ '/tasks/manage/fastdp#mtu' | relative_url }}) for more
  details.
* `NO_MASQ_LOCAL` - set to 0 to disable preserving the client source IP address when
  accessing Service annotated with `service.spec.externalTrafficPolicy=Local`.
  This feature works only with Weave IPAM (default).
* `IPTABLES_BACKEND` - set to `nft` to use `nftables` backend for `iptables` (default is `iptables-legacy`)

## <a name="securing-the-setup"></a> Securing the Setup

You should set the environment variable `WEAVE_PASSWORD` as stated in the previous section to enable the data plane encryption; 
this is a recommended option in case you cannot be sure about the security of the fabric between your nodes.

A different option is to use `trusted-subnets` and whitelist only the subnets that host your k8s nodes. Mind that depending on your circumstances that might allow a malicious container running in your cluster to access the weave dataplane, still.

Read on the [Securing Connections Across Untrusted Networks]({{ '/tasks/manage/security-untrusted-networks' | relative_url }}) document to see the alternatives.

To improve security drop `CAP_NET_RAW` from pod capabilities: by default pods can forge packets from anywhere on the network, which enables attacks such as DNS spoofing.

```
     securityContext:
       capabilities:
         drop: ["NET_RAW"]
```
