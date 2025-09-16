# Changelog

All changes made to the weave net codebase since the fork will be documented in this file.

## 2.9.0

### Changed

* Changed version in `reweave/Makefile` to 2.9.0
* Modified reweave and main CHANGELOG.md

## 2.9.0-beta1

From this version onwards, we will assume that the default backend for `iptables` is `iptables-nft`. For backward compatibility, we will include `iptables-legacy` in the weave images, and provide a way to use that if required.

### Changed

* Upgraded golang.org/x/crypto to v0.31.1
* Upgraded golang.org/x/net to v0.33.0
* Which upgraded:
	golang.org/x/mod v0.17.0
    golang.org/x/sync v0.10.0
	golang.org/x/term v0.27.0
	golang.org/x/text v0.21.0
* Ran `go mod tidy` and `go mod vendor`

### Added

* The Dockerfile was modified to keep the default iptables backend (nft) for all Alpine-based images, and to include the legacy backend but not set it as default.
* The `weave` script was modified to accept and propogate a new environment variable called `IPTABLES_BACKEND`, whose default value is `nft`. If its value is set to `legacy`, the script will use the command `iptables-legacy` instead of `iptables` when invoking that tool.
* The `launch.sh` script, which is the entry point for the `weave-kube` and `weave-npc` images, was modified to assume the `nft` backend as default, and to change the backend only if legacy mode was autodiscovered or set via the existing environment variable `IPTABLES_BACKEND`.

## 2.8.10

### Changed

* Changed version in `reweave/Makefile` to 2.8.10
* Modified reweave and main CHANGELOG.md

## 2.8.10-beta2

### Changed

* Changed go version in `go.mod` to 1.23.1
* Changed go base image in `reweave/build/Dockerfile` to `golang:1.23.1-bullseye`.


## 2.8.10-beta1

### Changed

* Changed the Alpine base image in `reweave/Makefile` and `reweave/build/Dockerfile` to `alpine:3.20.3`

## 2.8.9

### Changed

* Changed version in `reweave/Makefile` to 2.8.9
* Modified reweave and main CHANGELOG.md

## 2.8.9-beta1

### Changed

* Changed go base image in `reweave/build/Dockerfile` to `golang:1.22.2-bullseye`.
* Changed the Alpine base image in `reweave/Makefile` and `reweave/build/Dockerfile` to `alpine:3.20.2`
* Upgraded github.com/docker/docker to v25.0.6+incompatible
* Ran `go mod tidy` and `go mod vendor`

## 2.8.8

* Changed version in `reweave/Makefile` to 2.8.8
* Modified reweave and main CHANGELOG.md

## 2.8.8-beta1

### Changed

* Alpine base image upgraded to alpine:3.20.0 in `reweave/Makefile`

## 2.8.7

### Changed

* Changed version in `reweave/Makefile` to 2.8.6
* Modified reweave and main CHANGELOG.md

## 2.8.7-beta1

### Changed

* Upgraded golang.org/x/net to v0.23.0, which: 
* Upgraded golang.org/x/term to v0.18.0
* Upgraded golang.org/x/sys to v0.18.0
* Upgraded golang.org/x/crypto to v0.21.0
* Ran `go mod tidy` and `go mod vendor`
* Removed support for ppc64le and s90x architectures, because libpcap-0.8 in the debian buster repository does not provide binaries for those architectures any more

## 2.8.6

### Changed

* Changed version in `reweave/Makefile` to 2.8.6
* Modified reweave and main CHANGELOG.md
* Modified `reweave/Makefile` to ensure that `make build-plugin` builds the plugin only for the host architecture
* Made some documentation changes

## 2.8.6-beta1

### Changed

* Changed the V2 plugin build and clean tools to build (and clean) multiple architecture-specific plugins per version, since Docker Managed Plugins are not multi-architecture capable
* Changed the V2 plugin configuration `prog/net-plugin/config.json` to include `CHECKPOINT_DISABLE` as an option, set to `1` by default to disable latest version checking.
* Changed the `weave` script to include `export CHECKPOINT_DISABLE=`. This will disable latest version checking for the legacy plugin as well.
* Changed the documentation in the `website` directory as appropriate.

## 2.8.5

### Changed

* Changed version in `reweave/Makefile` to 2.8.5
* Modified reweave and main CHANGELOG.md

## 2.8.5-beta1 (ac180cfc)

### Changed

* Upgraded github.com/docker/docker to 24.0.9+incompatible
* Ran `go mod vendor`
* Modified `reweave/build/Dockerfile`, to cater for timeouts and other errors during `apt` operations

## 2.8.4 (c3afe552)

### Changed

* Changed version in `reweave/Makefile` to 2.8.4
* Modified reweave and main CHANGELOG.md
* Modified README.md

## 2.8.4-beta3 (8c148120)

### Changed

* Upgraded google.golang.org/protobuf to v1.33.0
* Changed the scan process to exclude weavedb and network-tester images. Weavedb is data-only, and network-tester does not get used outside of testing

## 2.8.4-beta2 (c1993b9d)

### Changed

* Changed the CNI version configured in the `weave` script to `1.0.0`, as per [this](https://www.cni.dev/docs/spec/#configuration-format)
* Ensured that the weave version gets added to `weaveutil` via linker flag
* Moved old, not-in-use artifacts to a directory called original
* Copied contents of old site directory into a directory called website. Rearranged and modified them for use with the Jekyll static site generator, for deployment on GitHub pages
* Added a GitHub actions workflow to build and publish the contents of the website directory

## 2.8.4-beta1 (bcab10a4)

### Changed
* Added tracing to `launch.sh` and `init.sh` scripts if the WEAVE_DEBUG environment variable is set.
* When publishing images, the `:latest` tag is also applied. It will not be applied any more if the published tag includes "-beta" anywhere.

### Fixed

* The Alpine Linux base image had been upgraded to 3.19.1. In this version, the default iptables backend is nftables, and the legacy backend is not included by default. Our scripts and programs assume legacy as the default backend, and change to nft if autodetected, or if we ask for it. So, changed our build script to install the Alpine `iptables-legacy` package, and changed the symbolic links to point to the legacy backend by default.

## 2.8.3 (d0878790)

### Changed

* Changed version in `reweave/Makefile` to 2.8.3
* Modified reweave and main CHANGELOG.md
* Modified the `weave` script

## 2.8.3-beta1 (a752f656)

### Changed

* The docker API client version, used by the proxy package and the weaveutil command, was bumped from 1.18 to 1.24. As of March 2024, Docker API versions below 1.24 are deprecated. This means that the minimum supported Docker version is now 1.12.0
* The scan report now scans all images other than the V2 Docker plugin

### Added

* Provision was made in weaveutil program and the weave script to override the API version used, via the environment variable `DOCKER_API_VERSION`. The same variable is used by standard docker clients

### Fixed

* Fixed `proxy.go` in the proxy package to handle changes in the Docker API. In the 1.24 API, container objects expose a `.Mounts` property rather than a `.Volumes` property.
* The `weave` script was modified to add a `-t` switch when invoking functionality inside a container. This ensures that output is visible even when using the Weave docker proxy

## 2.8.2 (7b087168)

### Changed

* Changed version in `reweave/Makefile` to 2.8.2
* Modified reweave and main CHANGELOG.md

## 2.8.2-beta15 (a9d66343)

### Changed

* Bumped go version in `go.mod` to `1.21`.
* Changed go base image in `reweave/build/Dockerfile` to `golang:1.21.6-bullseye`.

## 2.8.2-beta14 (0e7b15b3)

### Changed

* Vulnerability scanning output changed to a consolidated report
* Documentation changed to reflect new fork status

## 2.8.2-beta13 (c1d31074)

### Changed

* Module name changed to `github.com/rajch/weave`
* Added environment variable `CHECKPOINT_DISABLE=1` to default manifests, to bypass weaveworks telemetry

## 2.8.2-beta12 (962bb57b)

### Changed

* Alpine base image upgraded to alpine:3.19.1
* Upgraded github.com/containerd/containerd to v1.7.11
* Upgraded github.com/opencontainers/runc to v1.1.12
* Upgraded golang.org/x/crypto to v0.17.0

## 2.8.2-beta11 (0d58e179)

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
