---
title: Integrating Kubernetes via the Addon
menu_order: 20
search_type: Documentation
---

The following topics are discussed:

* [Installation](#install)
 * [Upgrading Kubernetes to version 1.6](#kube-1.6-upgrade)
 * [Upgrading the Daemon Sets](#daemon-sets)
 * [CPU and Memory Requirements](#resources)
 * [Pod Eviction](#eviction)
* [Features](#features)
 * [Pod Network](#pod-network)
 * [Network Policy](#npc)
* [Troubleshooting](#troubleshooting)
 * [Troubleshooting Blocked Connections](#blocked-connections)
 * [Things to watch out for](#key-points)
* [Changing Configuration Options](#configuration-options)


## <a name="install"></a> Installation

Weave Net can be installed onto your CNI-enabled Kubernetes cluster
with a single command:

```
$ kubectl apply -f "https://cloud.weave.works/k8s/net?k8s-version=$(kubectl version | base64 | tr -d '\n')"
```

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

**Note:** If using the [Weave CNI
Plugin](/site/kubernetes.md) from a prior full install of Weave Net with your
cluster, you must first uninstall it before applying the Weave-kube addon.
Shut down Kubernetes, and _on all nodes_ perform the following:

 * `weave reset`
 * Remove any separate provisions you may have made to run Weave at
   boot-time, e.g. `systemd` units
 * `rm /opt/cni/bin/weave-*`

Then relaunch Kubernetes and install the addon as described
above.

## <a name="kube-1.6-upgrade"></a> Upgrading Kubernetes to version 1.6

In version 1.6, Kubernetes has increased security, so we need to
create a special service account to run Weave Net. This is done in
the file `weave-daemonset-k8s-1.6.yaml` attached to the [Weave Net
release](https://github.com/weaveworks/weave/releases/latest).

Also, the
[toleration](https://github.com/kubernetes/community/blob/master/contributors/design-proposals/taint-toleration-dedicated.md)
required to let Weave Net run on master nodes has moved from an
annotation to a field on the DaemonSet spec object.

If you have edited the Weave Net DaemonSet from a previous release,
you will need to re-make your changes against the new version.

### <a name="daemon-sets"></a> Upgrading the Daemon Sets

For Kubernetes 1.6 and above the DaemonSet definition specifies
[Rolling Updates](https://kubernetes.io/docs/tasks/manage-daemon/update-daemon-set/),
so when you apply a new version Kubernetes will automatically restart
the Weave Net pods one by one.

Kubernetes v1.5 and below does not support rolling upgrades of daemon sets,
and so you will need to perform the procedure manually:

* Apply the updated addon manifest `kubectl apply -f "https://cloud.weave.works/k8s/net?k8s-version=$(kubectl version | base64 | tr -d '\n')"`
* Kill each Weave Net pod with `kubectl delete` and then wait for it to reboot before moving on to the next pod.

**Note:** In versions prior to Weave Net 2.0, deleting all Weave Net pods at the same time
  will result in them losing track of IP address range ownership, possibly leading to
  duplicate IP addresses if you then start a new copy of Weave Net.

## <a name="resources"></a>CPU and Memory Requirements

Kubernetes manages
[resources](https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/)
on each node, and only schedules pods to run on nodes that have enough
free resources.

The components of a typical Kubernetes installation (with the master
node running etcd, scheduler, api-server, etc.) take up about 95% of a
CPU, which leaves little room to run anything else. For all of Weave
Net's features to work, it must run on every node, including the
master.

The best way to resolve this issue is to use machines with at least
two CPU cores. However if you are installing Kubernetes and Weave Net
for the first time, you may not be aware of this requirement. For this
reason, Weave Net launches as a DaemonSet with a specification that
reserves at least 1% CPU for each container. This enables Weave Net to
start up seamlessly on a single-CPU node.

Depending on the workload, Weave Net may need more than 1% of the
CPU. The percentage set in the DaemonSet is the minimum and not a
limit. This minimum setting allows Weave Net to take advantage of
available CPU and "burst" above that limit if it needs to.

## <a name="eviction"></a>Pod Eviction

If a node runs out of CPU, memory or disk, Kubernetes may [decide to
evict](https://kubernetes.io/docs/concepts/cluster-administration/out-of-resource/)
one or more pods. It may choose to evict the Weave Net pod, which will
disrupt pod network operations.

You can reduce the chance of eviction by changing the DaemonSet to
have a much bigger request, and a limit of the same value.

This causes Kubernetes to apply a ["guaranteed" rather than a
"burstable" policy](https://github.com/kubernetes/community/blob/master/contributors/design-proposals/resource-qos.md).
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
definition](https://v1-7.docs.kubernetes.io/docs/api-reference/v1.7/#networkpolicy-v1-networking)

**Note:** as of version 1.9 of Weave Net, the Network Policy
  Controller allows all multicast traffic. Since a single multicast
  address may be used by multiple pods, we cannot implement rules to
  isolate them individually.  You can turn this behaviour off (block
  all multicast traffic) by adding `--allow-mcast=false` as an
  argument to `weave-npc` in the YAML configuration.

## <a name="troubleshooting"></a> Troubleshooting

Many Kubernetes network issues occur at a higher level than Weave Net.
The [Kubernetes Service Debugging Guide]
(https://kubernetes.io/docs/tasks/debug-application-cluster/debug-service/)
has a detailed step-by-step guide.

The status of Weave Net can be checked by running its CLI commands. This can be done in various ways:

1\. [Install the `weave` script](/site/installing-weave.md) and run:

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

3\. Finally you could also use [Weave Cloud](https://cloud.weave.works/) and monitor all your pods, including Weave Net's ones, from there.

![Weave Net status screen in Weave Cloud](weave-cloud-net-status.png)

For more information see [What is Weave Cloud?](https://www.weave.works/docs/cloud/latest/overview/)

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

- Don't turn on `--masquerade-all` on kube-proxy: this will change the
  source address of every pod-to-pod conversation which will make it
  impossible to correctly enforce network policies that restrict which
  pods can talk.
- If you do set the `--cluster-cidr` option on kube-proxy, make sure
  it matches the `IPALLOC_RANGE` given to Weave Net (see below)

## <a name="configuration-options"></a> Changing Configuration Options

#### Using `cloud.weave.works`

You can customise the YAML you get from `cloud.weave.works` by passing some of Weave Net's options, arguments and environment variables as query parameters:

  - `version`: Weave Net's version. Default: `latest`, i.e. latest release. *N.B.*: This only changes the specified version inside the generated YAML file, it does not ensure that the rest of the YAML is compatible with that version. To freeze the YAML version save a copy of the YAML file from the [release page](https://github.com/weaveworks/weave/releases) and use that copy instead of downloading it each time from `cloud.weave.works`.
  - `password-secret`: name of the Kubernetes secret containing your password.  *N.B*: The Kubernetes secret name must correspond to a name of a file containing your password.
     Example:

        $ echo "s3cr3tp4ssw0rd" > /var/lib/weave/weave-passwd
        $ kubectl create secret -n kube-system generic weave-passwd --from-file=/var/lib/weave/weave-passwd
        $ kubectl apply -f "https://cloud.weave.works/k8s/net?k8s-version=$(kubectl version | base64 | tr -d '\n')&password-secret=weave-passwd"

  - `known-peers`: comma-separated list of hosts. Default: empty.
  - `trusted-subnets`: comma-separated list of CIDRs. Default: empty.
  - `disable-npc`: boolean (`true|false`). Default: `false`.
  - `env.NAME=VALUE`: add environment variable `NAME` and set it to `VALUE`.
  - `seLinuxOptions.NAME=VALUE`: add SELinux option `NAME` and set it to `VALUE`, e.g. `seLinuxOptions.type=spc_t`
  - `use-legacy-netpol`: use [legacy NetworkPolicy semantics](https://v1-6.docs.kubernetes.io/docs/api-reference/v1.6/#networkpolicy-v1beta1-extensions), boolean (`true|false`). Default: `true` for Kubernetes version <= 1.6, `false` for > 1.6.

The list of variables you can set is:

* `CHECKPOINT_DISABLE` - if set to 1, disable checking for new Weave Net
  versions (default is blank, i.e. check is enabled)
* `CONN_LIMIT` - soft limit on the number of connections between
  peers. Defaults to 30.
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
  Manager](/site/operational-guide/concepts.md#ip-address-manager)
  (defaults to consensus amongst the `KUBE_PEERS`)
* `WEAVE_EXPOSE_IP` - set the IP address used as a gateway from the
  Weave network to the host network - this is useful if you are
  configuring the addon as a static pod.
* `WEAVE_METRICS_ADDR` - address and port that the Weave Net
  daemon will serve Prometheus-style metrics on (defaults to 0.0.0.0:6782)
* `WEAVE_STATUS_ADDR` - address and port that the Weave Net
  daemon will serve status requests on (defaults to disabled)
* `WEAVE_MTU` - Weave Net defaults to 1376 bytes, but you can set a
  smaller size if your underlying network has a tighter limit, or set
  a larger size for better performance if your network supports jumbo
  frames - see [here](/site/tasks/manage/fastdp.md#mtu) for more
  details.
* `NO_MASQ_LOCAL` - set to 1 to preserve the client source IP address when
  accessing Service annotated with `service.spec.externalTrafficPolicy=Local`.
  The feature works only with Weave IPAM (default).

Example:
```
$ kubectl apply -f "https://cloud.weave.works/k8s/net?k8s-version=$(kubectl version | base64 | tr -d '\n')&env.WEAVE_MTU=1337"
```
This command -- notice `&env.WEAVE_MTU=1337` at the end of the URL -- generates a YAML file containing, among others:

```
[...]
          containers:
            - name: weave
[...]
              env:
                - name: WEAVE_MTU
                  value: '1337'
[...]
```

**Note**: The YAML file can also be saved for later use or manual editing by using, for example:
```
$ curl -fsSLo weave-daemonset.yaml "https://cloud.weave.works/k8s/net?k8s-version=$(kubectl version | base64 | tr -d '\n')"
```

#### Manually editing the YAML file

Whether you saved the YAML file served from `cloud.weave.works` or downloaded a static YAML file from our [releases page](https://github.com/weaveworks/weave/releases), you can manually edit it to suit your needs.

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
