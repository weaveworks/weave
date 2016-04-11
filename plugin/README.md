# Weave network driver extension for Docker and Kubernetes

This directory implements both a 
[Docker plugin](http://docs.docker.com/engine/extend/plugin_api/) to
integrate [Weave Net](http://weave.works/net/) with Docker, and a 
[Container Network Interface (CNI) plugin](https://github.com/appc/cni#cni---the-container-network-interface),
to integrate Weave Net with [Kubernetes](http://kubernetes.io/).

The Docker plugin runs automatically when you `weave launch`, provided your
Docker daemon is version 1.9 or newer.

More detail on the Docker plugin [here](http://docs.weave.works/weave/latest_release/plugin.html).

More detail on the CNI plugin [here](https://github.com/weaveworks/weave/blob/master/site/plugin/cni-plugin-how-to.md).
