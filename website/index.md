---
title: Re-introducing Weave Net
menu_order: 01
search_type: Documentation
---
Weave Net was the first product created by Weaveworks. It was and still is a popular choice for container networking, especially as a CNI add-on for Kubernetes.

Weave Net is now a community-supported project, run out of the forked GitHub repository [rajch/weave](https://github.com/rajch/weave).

## Using Weave with Kubernetes

If you are here for the most popular use of Weave Net - the famous one-line installation on Kubernetes, look no further. 

You can install Weave Net on a Kubernetes cluster using:

```bash
kubectl apply -f https://reweave.azurewebsites.net/k8s/v1.29/net.yaml
```
where the `v1.29` part can be replaced with any Kubernetes version, the minimum being `v1.8`.

If you prefer the older method using `kubectl version`, that works too:

```bash
KUBEVER=$(kubectl version | base64 | tr -d '\n')
kubectl apply -f https://reweave.azurewebsites.net/k8s/net?k8s-version=$KUBEVER
```

Finally, you can directly apply the manifest from our releases page:

```bash
kubectl apply -f https://github.com/rajch/weave/releases/latest/download/weave-daemonset-k8s-1.11.yaml
```

More details, and options can be found on the [Integrating Kubernetes via the Addon]({{ '/kubernetes/kube-addon' | relative_url }}) page.

## Using Weave as a (legacy) Docker Plugin

See [Installing Weave Net]({{ '/install/installing-weave' | relative_url }}), and then [Launching Weave Net]({{ '/install/using-weave' | relative_url }}).

## Using Weave as a Docker Managed (V2) Plugin in Swarm Mode

See [Integrating Docker via the Network Plugin (V2)]({{ '/install/plugin/plugin-v2' | relative_url }}).

**Note:** From version 2.8.6 onwards, the Docker V2 plugin is available for amd64, arm, arm64 ~~, ppc64le and s390x~~ architectures. Prior to this, it was only available for amd64.

## All other uses, and more

For all other uses, proceed to the [Weave Net overview]({{ '/overview' | relative_url }}).

## Documentation Status

This documentation is currently a work in progress. All content from the old Weaveworks site has been brought over, and is in the process of getting updated with all the changes made in this community-run fork. Please bear with us until it is complete.
