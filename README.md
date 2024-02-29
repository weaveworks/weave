# ReWeave - an effort to keep Weave Net alive

This repository contains a fork of Weave Net, the first product developed by Weaveworks.

## About Weaveworks

Weaveworks created many innovative products and services around containers, Kubernetes and the cloud. They were pioneers in CNI networking and GitOps.

## Weave Net

Weave Net creates a virtual network that connects containers across multiple hosts and enables their automatic discovery. With Weave Net, portable microservices-based applications consisting of multiple containers can run anywhere: on one host, multiple hosts or even across cloud providers and data centers. Applications use the network just as if the containers were all plugged into the same network switch, without having to configure port mappings, ambassadors or links. Weave Net is also available as a CNI plugin, which allows it to provide container networking on Kubernetes clusters.

## History of the ReWeave project

In June 2022, Weave Net had not been updated for a year. Problems were starting to appear in the field. In particular, the last published images on the Docker Hub had issues supporting multiple processor architectures, and security scanners showed multiple vulnerabilities. 

A call went out from Weaveworks to get the community more involved in maintaining it. After some discussion on GitHub issues and e-mail, and even a few online meetings, things were not moving forward. Finally, in March 2023, this fork was created, with the following goals in mind:

* Update dependencies, especially ones with security vulnerabilities
* Make minimal code changes _only_ when required by updating dependencies
* Create true multi-arch images using modern tools
* Create a new build process to automate all this

These goals were achieved. Details can be found in the [reweave](reweave/README.md) directory. A pull request was submitted on the weaveworks repo, with the aim of getting a new official release out.

On February 5th, 2024, Weaveworks CEO Alexis Richardson announced via [LinkedIn](https://www.linkedin.com/posts/richardsonalexis_hi-everyone-i-am-very-sad-to-announce-activity-7160295096825860096-ZS67/) and [Twitter](https://twitter.com/monadic/status/1754530336120140116) that Weaveworks is winding down. 

So, this fork will now be maintained independently. 

## New Goals

The old goals, listed above, remain the priority. In addition, this project aims to:

* Remove dependencies on Weaveworks infrastructure, starting with telemetry
* Publish new releases regularly
* Provide supporting infrastructure, such as weave's famous one-line installation, where possible
