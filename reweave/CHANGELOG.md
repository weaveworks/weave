# Changelog

All changes made to the weave net codebase during the reweave effort will be documented in this file.

## latest

### Changed

* Alpine base image upgraded to alpine:2.18.4, and `apk upgrade` applied
* Upgraded golang.org/x/net to v0.17.0. This upgraded some dependecies
* Upgraded github.com/docker/docker to v24.0.7. This caused a compilation error
* Upgraded github.com/fsouza/go-dockerclient to v1.10.0. This resolved the error 

## 2.8.2-beta10 (126f3ab9)

### Changed

* Alpine base image upgraded to alpine:2.18.3
* Updated documentation
* Changed [reweave/tool/build-images.sh](reweave/tool/build-images.sh) to be consistent with documentation

## 2.8.2-beta9 (c085ebed)

### Changed

* Ran `go mod tidy` to remove unused references
* Included tools and build steps to build the docker plugin (v2)

## 2.8.2-beta8 (668bbcf4)

### Changed

* Upgraded github.com/opencontainers/runc to v1.1.5
* Upgraded github.com/docker/docker to v23.0.3+incompatible
* Upgraded github.com/docker/distribution to v2.8.2-beta.1+incompatible

## 2.8.2-beta7 (4b47fc60)

### Changed

* Alpine base image was changed to `alpine:3.18.2`

## 2.8.2-beta6 (559027b4)

### Changed

* Alpine base image was changed to `alpine:3.17.3`

### Fixed

* Corrections were made in `reweave/build/Dockerfile` to ensure it builds everywhere, specifically on M1 Macs too
* `ALPINE_BASEIMAGE` was added as a parameter to `reweave/Makefile`. Now all changeable parameters are in one place

## 2.8.2-beta5 (88d8bb64)

Docker, containerd and runc dependencies were upgraded here.

### Changed

* Upgraded github.com/containerd/containerd to v1.7.0
* Upgraded github.com/opencontainers/runc to v1.1.4
* Upgraded github.com/docker/docker to v23.0.1+incompatible
* Upgraded github.com/docker/distribution to v2.8.1+incompatible

## 2.8.2-beta4 (1bb1f02b)

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

## 2.8.2-beta3 (2f49b845)

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

## 2.8.2-beta2 (df2240ac)

Some low-impact vulnerable dependencies were upgraded here

### Changed

* Upgraded golang.org/x/net to v0.8.0
* Upgraded golang.org/x/crypto to v0.0.0-20220314234659-1baeb1ce4c0b 
* Upgraded github.com/aws/aws-sdk-go to **v1.34.0**
* Upgraded github.com/prometheus/client_golang to **v1.14.0**
* Upgraded k8s.io/client-go to **v0.23.0**

## 2.8.2-beta1 (c241c455)

### Changed

* Bumped go version in `go.mod` to `1.20`. This caused an error in `go mod tidy`
* Manually ran `go get github.com/andybalholm/go-bit@v1.0.1`. `go mod tidy` and `go mod vendor` worked

## 2.8.2-beta0 (90a5d945)

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
