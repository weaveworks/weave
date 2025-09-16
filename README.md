# Weave Net

This repository contains a fork of Weave Net, the first product developed by Weaveworks. Since Weaveworks has shut down, this repo aims to continue maintaining Weave Net, and to publish releases regularly.

[![Go Report Card](https://goreportcard.com/badge/github.com/rajch/weave)](https://goreportcard.com/report/github.com/rajch/weave)
[![Docker Pulls](https://img.shields.io/docker/pulls/rajchaudhuri/weave-kube "Number of times the weave-kube image was pulled from the Docker Hub")](https://hub.docker.com/r/rajchaudhuri/weave-kube)
[![GitHub release (latest by date)](https://img.shields.io/github/v/release/rajch/weave?include_prereleases)](https://github.com/rajch/weave/releases)
[![Unique CVE count in all images](https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2Frajch%2Fweave%2Fmaster%2Freweave%2Fscans%2Fbadge.json&label=CVE%20count "The number of unique CVEs reported by scanning all images")](reweave/scans/report.md)

The history of the fork can be found in [HISTORY.md](HISTORY.md).

## Using Weave Net on Kubernetes

On a newly created Kubernetes cluster, the Weave Net CNI pluging can be installed by running the following command:

```
kubectl apply -f https://reweave.azurewebsites.net/k8s/v1.29/net.yaml
```

Replace `v1.29` with the version on Kubernetes on your cluster.

That endpoint is provided by the companion project [weave-endpoint](https://github.com/rajch/weave-endpoint).

## Using Weave Net in other ways

Please refer to the [documentation](https://rajch.github.io/weave).

## Building Weave Net

Details can be found [here](reweave/BUILDING.md). 

## Documentation status

The public documentation that used to exist in the `site` directory has been moved to the `original/site` directory. A new `website` directory has been created, and populated with the content of the `original/site` directory, rearranged and reformatted for being built with Jekyll and published to the GitHub pages site [https://rajch.github.io/weave](https://rajch.github.io/weave).

The documentation will now be maintained and published from the `website` directory exclusively.
