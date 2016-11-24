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

### Blocked connections

If you suspect that legitimate traffic is being blocked by the Weave Network Policy Controller, you may want to look at the `weave-npc` container's logs:
```
$ kubectl logs $(kubectl get pods --all-namespaces | grep weave-net | awk '{print $2}') -n kube-system weave-npc
```

Any time the Weave Network Policy Controller blocks a connection, it will log details about it:

* protocol used, 
* source IP and port, 
* destination IP and port, 

as per the below example:
```
time="yyyy-MM-ddTHH:mm:ssZ" level=warning msg="TCP connection from 10.32.0.7:56648 to 10.32.0.11:80 blocked by Weave NPC.
time="yyyy-MM-ddTHH:mm:ssZ" level=warning msg="UDP connection from 10.32.0.7:56648 to 10.32.0.11:80 blocked by Weave NPC.
```

## Changing Configuration Options

You can change the default configuration by saving and editing the
addon YAML before you `kubectl apply`. Additional arguments can be
supplied to the Weave router process by adding them to the `command:`
array in the yaml file.

Some parameters are changed by environment variables; these can be
inserted into the YAML file like this:

```
      containers:
        - name: weave
          env:
            - name: IPALLOC_RANGE
              value: 10.0.0.0/16
```

The list of variables you can set is:

* IPALLOC_RANGE - the range of IP addresses used by Weave Net
  and the subnet they are placed in (CIDR format; default 10.32.0.0/12)
* EXPECT_NPC - set to 0 to disable Network Policy Controller (default is on)
* KUBE_PEERS - list of addresses of peers in the Kubernetes cluster
  (default is to fetch the list from the api-server)
* IPALLOC_INIT - set the initialization mode of the [IP Address
  Manager](/site/operational-guide/concepts.md#ip-address-manager)
  (defaults to consensus amongst the KUBE_PEERS)
