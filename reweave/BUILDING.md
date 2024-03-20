# New Build Process

This new build process is based on a multi-stage Dockerfile, which combines the build image Dockerfile and all the template-generated final image Dockerfiles from the [old build process](BUILDING-OLD.md).

> NOTE: The new process currently does not ~~build the weave Docker plugin. Nor does it~~ run any tests. This may change in the future.

The new process is controlled by a Makefile, located at `reweave/Makefile`. This calls scripts located at `reweave/tools`. In the future, we should try to maintain this "harness", and only add/change the scripts as required. So, going forward, all anyone has to do is `cd reweave && make build && make scan`, no matter how the build process is evolved.

## What you need

* Docker v23.0.1 or later
* An account on an image registry, like the Docker Hub
* `make`
* [grype](https://github.com/anchore/grype) 0.59.0 or later for image scanning
* The `tools` directory contents. Run `git submodule update --init`.

## Build steps

### During development

At the start of each development cycle, edit [reweave/Makefile](Makefile) and set `IMAGE_VERSION` should be set to something later than the currently published version, perhaps with a prerelease suffix. Also, set `REGISTRY_USER` to *your* registry user account.

After you make code or configurations changes, run the following in the `reweave` directory:

```bash
make
```

This will build all weave net images for your local platform(single-architecture), tag them with your repo user name, and load them to your local docker engine. 

Once the images are built, you can scan the `weave-kube` and `weave-npc` images by running:

```bash
make scan
```

This will scan the images and generate reports in the [scans](scans/) directory.

### To publish images

1. Change `IMAGE_VERSION` and `REGISTRY_USER` variables in [reweave/Makefile](Makefile) to their final, publishable versions.
2. Build and scan as you would for development.
3. When satisfied, commit and tag your changes. **Do this before the next steps, because git metadata is picked from your repository.**.
4. Login to your registry account using `docker login`.
5. Run the following:

```bash
make publish
```

This will build multi-architecture images, and push them to your registry.

Don't forget to `docker logout` afterwards.

## Build Artifacts

### Multi-stage Dockerfile

The Dockerfile can be found at [reweave/build/Dockerfile](build/Dockerfile). It, and the companion [Makefile](#makefile), build multi-arch images for all weave components.

#### Stage 1 (builderbase)

The first stage creates a build environment for cross-compiling all weave net executables for all supported architectures. It is built only once for a given build platform. It starts with `golang:1.20.2-bullseye` as a base, and does the following:

* Installs build essentials and cross-compilers for all supported architectures.
* Adds the Debian buster repositories. This is because the last supported version of `libpcap` that allows static linking (1.8.1) is not present in the Debian bullseye repos.
* Installs `libpcap-dev`, pinned to v1.8.1.

#### Stage 2 (builder)

The second stage is where the weave net executables are built. It is built once for each target architecture.

It starts from `builderbase` as the base, and does the following:

* Sets environment variables indicating the current build and target architectures
* Copies in the source code
* Downloads modules
* Uses a [Makefile](build/Makefile) to build all weave net executables for the target architecture

#### Stage 3 (alpinebase)

All weave net images ultimately derive from an Alpine base image. This stage allows customization of an official Alpine image, and using the customized image as the base for the weave net images.

#### Subsequent stages

Each of the subsequent stages builds one image, corresponding to one component of weave net. These start from `alpinebase`, or `weaverimage` (Stage 4) as the base, and add what executables, scripts and configuration they need from `builder`.

### Makefile

The [Makefile](build/Makefile) is a distilled version of the [old Makefile](../Makefile). It only builds executables. It invokes the correct C cross-compiler, and picks up the correct static `libpcap`, based on the target architecture. **It is meant be used only in Stage 2 of the build process, and not directly on a development environment.**

## Build Tools

### reweave/Makefile

This makefile controls the build and scan processes. It provides the following parameters:

|Parameter|Description|Default value|
|---|---|---|
|IMAGE_VERSION|The tag part of the name (after `:`) for all weave net images.|*version set in makefile*|
|REGISTRY_USER|The account name (with optional registry name in front) used to tag all weave net images.|`weaveworks`|
|ALPINE_BASEIMAGE|The qualified name for the base Alpine image used to build all weave net images.|`alpine:3.18.2`|

and the following targets:

|Target|Description|
|---|---|
|build-images|Builds images matching the architecture of the local Docker engine, and loads it into the local Docker engine. This is the default target.|
|publish-images|Builds multi-architecture images for all configured architectures, and pushes them to the registry.|
|clean-images|Removes images from the local docker engine.|
|build-plugin|Builds the [weave Docker Network Plugin(v2)](https://www.weave.works/docs/net/latest/install/plugin/plugin-v2/) on the local Docker engine. Requires published images.|
|publish-plugin|Pushes the plugin to the registry.|
|clean-plugin|Removes the plugin from the local Docker engine.|
|publish|Combines the publish-images and publish-plugin steps, in that order.|
|clean|Combines the clean-images and clean-plugin steps, in that order.|
|scan|Scans the weave-kube and weave-npc images using the configured scanner (currently grype), and stores the results in `reweave/scans`.|
|clean-scan|Deletes scan results.|

### reweave/tools/build-images.sh

This script invokes `docker buildx build` for each stage from stage 4 onwards. By default, it builds for all supported platforms, tags the image as `weaveworks/IMAGENAME:CURRENTVERSION`, and keeps them in the build cache. This behavior can be controlled by setting the following environment variables before invoking the script.

|Env Var Name|Description|Default Value|
|---|---|---|
|IMAGE_VERSION|The tag part of the name (after `:`) for all weave net images.|*version set in Makefile*|
|REGISTRY_USER|The account name (with optional registry name in front) used to tag all weave net images.|*user set in Makefile*|
|ALPINE_BASEIMAGE|The qualified name for the base Alpine image used to build all weave net images.|`alpine:3.18.2`|
|PLATFORMS|Comma-separated list of the target platforms for which the weave net images will be built.|`linux/amd64,linux/arm,linux/arm64,linux/ppc64le,linux/s390x`|
|PUBLISH|Whether to push the images after build (`true`) , or load them to the local Docker engine (`false`). `false` is only possible if PLATFORMS has the same value as the build platform. If left empty, the images will be built in the build cache only.||

### reweave/tools/clean-images.sh

This script deletes the built images from the local Docker engine.

### reweave/tools/build-plugin.sh

This script builds the weave managed network plugin on the local Docker engine. It requires the following environment variables to be set:

|Env Var Name|Description|Default Value|
|---|---|---|
|IMAGE_VERSION|The tag part of the name (after `:`) for the plugin.|*version set in Makefile*|
|REGISTRY_USER|The account name (with optional registry name in front) used to tag all weave net images.|*user set in Makefile*|
|PUBLISH|Whether to push the plugin to the Docker Hub after build (`true`) , or not (`false` or empty).||

### reweave/tools/clean-plugin.sh

This script deletes the weave managed plugin from the local Docker engine.

### reweave/tools/scan-images.sh

This script scans the weave-kube and weave-npc images using [grype](https://github.com/anchore/grype). It saves scan results in the directory `reweave/scans`.

### reweave/tools/clean-scans.sh

This script clears the `reweave/scans` directory.
