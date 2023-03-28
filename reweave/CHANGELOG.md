# Changelog

All changes made to the weave net codebase during the reweave effort will be documented in this file.

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
