---
title: Integrating Kubernetes via the Addon
menu_order: 63
---

## Installation

If you're using Kubernetes 1.4 or later, and it is configured to use
CNI, it is possible to install Weave Net on your cluster with a single
command:

```
kubectl apply -f https://git.io/weave-kube
```

After a few seconds, you should have a Weave Net pod running on each
node, and any further pods you create will be attached to the Weave
network.

> If you do not already have a CNI-enabled cluster, you can bootstrap
> one easily with
> [kubeadm](http://kubernetes.io/docs/getting-started-guides/kubeadm/);
> alternatively if you are using the older cluster set-up scripts from
> the Kubernetes repo, you can use
>
> ```
> NETWORK_PROVIDER=cni cluster/kube-up.sh
> ```

NB If you were previously using the [Weave CNI
driver](/site/cni-plugin.md) from a full install of Weave Net with your
cluster, you will need to uninstall it first. Shut down Kubernetes,
then perform the following _on all nodes_:

 * `weave reset`
 * Remove any separate provision you have made to run weave at
   boot-time, e.g. `systemd` units
 * `rm /opt/cni/bin/weave-*`

You can then relaunch Kubernetes and install the addon as described
above.

The URL [https://git.io/weave-kube](https://git.io/weave-kube) points
to the addon YAML for the latest release of the Weave Net addon;
historic versions will be archived on our [GitHub release
page](https://github.com/weaveworks/weave/releases).

## Upgrading

Kubernetes does not currently support rolling upgrades of daemon sets,
so you will need to perform the procedure manually:

* Apply the updated addon manifest `kubectl apply -f https://git.io/weave-kube`
* Kill each weave net pod in turn with kubectl delete; wait for the
  replacement to begin running before moving on to the next.

##<a name="npc"></a>Network Policy Controller

The addon supports the [Kubernetes policy
API](http://kubernetes.io/docs/user-guide/networkpolicies/) so that
you can securely isolate pods from each other based on namespaces and
labels. For more information on configuring network policies in
Kubernetes see the
[walkthrough](http://kubernetes.io/docs/getting-started-guides/network-policy/walkthrough/)
and the [NetworkPolicy API object
definition](http://kubernetes.io/docs/api-reference/extensions/v1beta1/definitions/#_v1beta1_networkpolicy).

## Changing Configuration Options

You can change the default configuration by saving and editing the
addon YAML before you `kubectl apply`. Additional arguments can be
supplied to the Weave router process by adding them to the `command:`
array in the yaml file.
