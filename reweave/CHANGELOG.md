# Changelog

All changes made to the weave net codebase during the reweave effort will be documented in this file.

## 2.8.2-beta5

Docker, containerd and runc dependencies were upgraded here.

### Changed

* Upgraded github.com/containerd/containerd to v1.7.0
* Upgraded github.com/opencontainers/runc to v1.1.4
* Upgraded github.com/docker/docker to v23.0.1+incompatible
* Upgraded github.com/docker/distribution to v2.8.1+incompatible

## 2.8.2-beta4

CNI, which introduced breaking changes, was upgraded here. The majority of this work was already done in the unmerged pull request [3939](https://github.com/weaveworks/weave/pull/3939) by by @hswong3i. All credit to them

### Changed

* Upgraded github.com/containernetworking/cni to v1.1.2

### Added

* Added github.com/containernetworking/plugins, v1.2.0

### Fixed

* Changed `plugin/ipam/cni.go` and `plugin/net/cni.go` as per [CNI spec-upgrades](https://www.cni.dev/docs/spec-upgrades/)
* Changed `prog/weaveutil/cni.go` to accommodate above changes
* Changed `prog/kube-utils/main.go` due to an observed change in behavior
* Changed `weave` script in project root, to add `disableCheck: true` in cni config. This is as per information found [here](https://www.cni.dev/docs/spec-upgrades/#changes-in-cni-v04) and [here](https://www.cni.dev/docs/spec/#configuration-format)

## 2.8.2-beta3

github.com/miekg/dns was upgraded here

### Changed

* Upgraded github.com/miekg/dns to v1.1.52. This caused the following build error:
```
../../nameserver/dns.go:276:32: undefined: dns.ErrTruncated
```

### Fixed

* Found references to the above error [here](https://github.com/miekg/dns/issues/814) and [here](https://github.com/miekg/dns/issues/423)
 * Deleted check for ErrTruncated in nameserver/dns.go:276:32
 * Modified nameserver/dns_test.go to remove dns.ErrTruncated-based tests

## 2.8.2-beta2

Some low-impact vulnerable dependencies were upgraded here

### Changed

* Upgraded golang.org/x/net to v0.8.0
* Upgraded golang.org/x/crypto to v0.0.0-20220314234659-1baeb1ce4c0b 
* Upgraded github.com/aws/aws-sdk-go to **v1.34.0**
* Upgraded github.com/prometheus/client_golang to **v1.14.0**
* Upgraded k8s.io/client-go to **v0.23.0**

## 2.8.2-beta1

### Changed

* Bumped go version in `go.mod` to `1.20`. This caused an error in `go mod tidy`
* Manually ran `go get github.com/andybalholm/go-bit@v1.0.1`. `go mod tidy` and `go mod vendor` worked

## 2.8.2-beta0

A new build process was created here.

### Added

* Documentation and tools under the `reweave` directory
* .dockerignore in the repository root, to exclude files from the new build process
* Multi-stage Dockerfile, and a Makefile for the second stage, in `reweave/build`

### Changed
* Included image building targets in Makefile directly under `reweave`
* Changed `IMAGE_VERSION` to `2.8.2-beta0` in Makefile directly under `reweave`
* The Alpine base image for all weave net images was set to `alpine:3.17.2` via the multi-stage Dockerfile
* Re-generated security scan reports after building with the new process

## 2.8.1

This was the last released version of the weave net images before the reweave effort began. 

### Added

* Documentation and tools under the `reweave` directory
* Image security scanning tools
* Security scan reports for `weave-kube` and `weave-npc` images
