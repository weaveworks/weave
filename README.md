# ReWeave - an effort to keep Weave Net alive

This repository contains a fork of Weave Net, the first product developed by Weaveworks. Since Weaveworks has shut down, this repo aims to continue maintaining Weave Net, and to publish releases regularly.

[![Go Report Card](https://goreportcard.com/badge/github.com/rajch/weave)](https://goreportcard.com/report/github.com/rajch/weave)
[![Docker Pulls](https://img.shields.io/docker/pulls/rajchaudhuri/weave-kube)](https://hub.docker.com/r/rajchaudhuri/weave-kube)
![GitHub release (latest by date)](https://img.shields.io/github/v/release/rajch/weave?include_prereleases)
[![Unique vulnerability count in all images](https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2Frajch%2Fweave%2Fmaster%2Freweave%2Fscans%2Fbadge.json&label=Vulnerabilty%20count)](reweave/scans/report.md)

The history of the ReWeave effort can be found in [HISTORY.md](HISTORY.md).

## Using Weave on Kubernetes

On a newly created Kubernetes cluster, the Weave Net CNI pluging can be installed by running the following command:

```
kubectl apply -f https://reweave.azurewebsites.net/k8s/v1.28/net.yaml
```

Replace `v1.28` with the version on Kubernetes on your cluster.

That endpoint is provided by the companion project [weave-endpoint](https://github.com/rajch/weave-endpoint).

## Building Weave

Details can be found [here](reweave/BUILDING.md). 

## Documentation status

At this point, any information found in directories other than `reweave`, such as `docs` or `site`, should be considered obsolete. In time, those will be updated.
