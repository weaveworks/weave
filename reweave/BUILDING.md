# New Build Process

This new build process is based on a multi-stage Dockerfile, which combines the build image Dockerfile and all the template-generated final image Dockerfiles from the [old build process](BUILDING-OLD.md).

> NOTE: The new process currently does not build the weave Docker plugin. Nor does it run any tests. This may change in the future.

## What you need

* Docker v23.0.1 or later
* An account on an image registry, like the Docker Hub
* [grype](https://github.com/anchore/grype) 0.59.0 or later for image scanning
* The `tools` directory contents. Run `git submodule update --init`.

## During development

At the start of each development cycle, edit [reweave/Makefile](Makefile) and set `IMAGE_VERSION` should be set to something later than the currently published version, perhaps with a prerelease suffix. Also, set `REGISTRY_USER` to a your registry user account.

After you make code or configurations changes, run the following in the `reweave` directory:

```bash
make
```

This will build all weave net images for your local platform, tag them with your repo user name, and load them to your local docker engine. 

Once the images are built, you can scan the `weave-kube` and `weave-npc` images by running:

```bash
make scan
```

This will scan the images and generate reports in the [scans](scans/) directory.

## To publish images

1. Change `IMAGE_VERSION` and `REGISTRY_USER` variables in [reweave/Makefile](Makefile) to their final, publishable versions.
2. Build and scan as you would for development.
3. Commit and tag your changes.
4. Login to your registry account using `docker login`.
5. Run the following:

```bash
make publish
```

This will build multi-architecture images, and push them to your registry.

Don't forget to `docker logout` afterwards.

## Multi-stage Dockerfile

The Dockerfile can be found at [reweave/build/Dockerfile](build/Dockerfile). It, and the companion [Makefile](#makefile), build multi-arch images for all weave components.

### Stage 1 (builderbase)

The first stage creates a build environment for cross-compiling all weave net executables for all supported architectures. It is built only once for a given build platform. It starts with `golang:1.20.2-bullseye` as a base, and does the following:

* Installs build essentials and cross-compilers for all supported architectures.
* Adds the Debian buster repositories. This is because the last supported version of `libpcap` that allows static linking (1.8.1) is not present in the Debian bullseye repos.
* Installs `libpcap-dev`, pinned to v1.8.1.

### Stage 2 (builder)

The second stage is where the weave net executables are built. It is built once for each target architecture.

It starts from `builderbase` as the base, and does the following:

* Sets environment variables indicating the current build and target architectures
* Copies in the source code
* Downloads modules
* Uses the [Makefile](#makefile) to build all weave net executables for the target architecture

### Stage 3 (alpinebase)

All weave net images ultimately derive from an Alpine base image. This stage allows customization of an official Alpine image, and using the customized image as the base for the weave net images. Currently, there is no customization required.

### Subsequent stages

Each of the subsequent stages builds one image, corresponding to one component of weave net. These start from `alpinebase`, or `weaverimage` (Stage 4) as the base, and add what executables, scripts and configuration they need from `builder`.

## Makefile

The [Makefile](build/Makefile) is a distilled version of the [old Makefile](../Makefile). It only builds executables. It invokes the correct C cross-compiler, and picks up the correct static `libpcap`, based on the target architecture. It is meant be used only in Stage 2 of the build process, and not directly.

## build-images.sh

This script invokes `docker buildx build` for each stage from stage 4 onwards. By default, it builds for all supported platforms, tags the image as `weaveworks/IMAGENAME:CURRENTVERSION`, and keeps them in the build cache. This behavior can be controlled by setting the following environment variables before invoking the script.

|Env Var Name|Description|Default Value|
|---|---|---|
|IMAGE_VERSION|The tag part of the name (after `:`) for all weave net images.|*version set in script file*|
|REGISTRY_USER|The account name (with optional registry name in front) used to tag all weave net images.|`weaveworks`|
|PLATFORMS|Comma-separated list of the target platforms for which the weave net images will be built.|`linux/amd64,linux/arm,linux/arm64,linux/ppc64le,linux/s390x`|
|POSTBUILD|Whether to push the images after build (`--push`) , or load them to the local Docker engine (`--load`). `--load` is only possible if PLATFORMS has the same value as the build platform. If left empty, the images will be built in the build cache only.||
|ALPINE_BASEIMAGE|The qualified name for the base Alpine image used to build all weave net images.|`alpine:3.17.2`|
