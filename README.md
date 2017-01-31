# Weave Net - Weaving Containers into Applications

[![Integration Tests](https://circleci.com/gh/weaveworks/weave/tree/master.svg?style=shield)](https://circleci.com/gh/weaveworks/weave)
[![Coverage Status](https://coveralls.io/repos/weaveworks/weave/badge.svg)](https://coveralls.io/r/weaveworks/weave)
[![Go Report Card](https://goreportcard.com/badge/github.com/weaveworks/weave)](https://goreportcard.com/report/github.com/weaveworks/weave)
[![Slack Status](https://slack.weave.works/badge.svg)](https://slack.weave.works)
[![Docker Pulls](https://img.shields.io/docker/pulls/weaveworks/weave.svg?maxAge=604800)](https://hub.docker.com/r/weaveworks/weave/)

# About Weaveworks

[Weaveworks](https://www.weave.works) is the company that delivers the most productive way for developers to connect, observe and control
Docker containers.

This repository contains [Weave Net](https://www.weave.works/products/weave-net/), the first product developed by Weaveworks, and with over 8 million downloads to date, it enables you to get started with Docker clusters and portable apps in a fraction of the time compared with other solutions.

Weaveworks' product [Weave Cloud](https://www.weave.works/solution/cloud/) takes the heavy lifting out of building, integrating and operating open source components, so you don’t have to manage databases, storage and availability.

Weave Cloud is built using these Open Source projects: [Weave Scope](https://www.weave.works/products/weave-scope/), a powerful container monitoring tool that automatically maps Docker containers and their interactions, [Weave Cortex](https://github.com/weaveworks/cortex), a horizontally-scalable version of Prometheus, and [Weave Flux](https://www.weave.works/products/weave-flux/), a continuous deployment tool that works with Kubernetes.

# Weave Net

Weave Net creates a virtual network that connects Docker containers across multiple hosts and enables their automatic discovery. With Weave Net, portable microservices-based applications consisting of multiple containers can run anywhere: on one host, multiple hosts or even across cloud providers and data centers. Applications use the network just as if the containers were all plugged into the same network switch, without having to configure port mappings, ambassadors or links.

Services provided by application containers on the Weave network can be exposed to the outside world, regardless of where they are running. Similarly, existing internal systems can be opened to accept connections from application containers irrespective of their location.

* [Install Weave Net](https://www.weave.works/docs/net/latest/installing-weave/)
* [Find out more about Weave Net](https://www.weave.works/products/weave-net/)
