# Build process as of commit 8c847638, slightly after v2.8.1

The build process is controlled by the Makefile found in the root of the module. This builds various weave net executables, and also builds docker images. It uses a complex process for building multi-architecture docker images, involving cross-compilation, architecture-wise image naming, and the [manifest tool](https://github.com/estesp/manifest-tool). Here are some observations.

Several parts of the Makefile assume that the build machine's architecture is amd64.

The makefile can build things either on the host machine, or in a docker container. The Makefile parameter `BUILD_IN_CONTAINER` controls this. If chosen, the build container itself runs Docker in Docker, bind mounts the source directory, and builds using the same Makefile.

The build process requires `CGO_ENABLED=1`, and needs the presence of an architecture-specific C compiler. It also needs an architecture-specific `libpcap.a` to be available in the build machine. If building outside of a container, all this has to be provided ahead of time. If inside, the Makefile can build a build container image which contains all this, and then create a container from that image and run the rest of the build process in that container.

The build container image is built from the following directory:

* build/

The build container image is created from a Debian buster-based base image. It installs the `libpcap-dev` package for the build platform (assumed to be amd64). It then installs cross-compilers for other supported architectures(arm, arm64, ppc64le, s390x), downloads libpcap source code (version 1.6.2) and compiles it for each architecture. Details can be found in the build container image Dockerfile, at [build/Dockerfile](../build/Dockerfile).

Either way, the build process builds the following executables:

* prog/weaver/weaver
* prog/sigproxy/sigproxy
* prog/kube-utils/kube-utils
* prog/weave-npc/weave-npc
* prog/weavewait/weavewait
* prog/weavewait/weavewait_noop
* prog/weavewait/weavewait_nomcast
* prog/weaveutil/weaveutil
* tools/runner/runner
* test/tls/tls
* test/images/network-tester/webserver

After building these, the build process builds a number of Docker images. To do this, it generates per-architecture Dockerfiles from the following "template" files:

* prog/weave-kube/Dockerfile.template
* prog/weave-npc/Dockerfile.template
* prog/weaveexec/Dockerfile.template
* prog/weaver/Dockerfile.template

The templates work as follows:

For `prog/weaver/Dockerfile.template`, the template fills in an Alpine base image. The image is controlled by the Makefile parameter `ALPINE_BASEIMAGE`. When building for a non-amd64 architecture, the template adds a `COPY` instruction to copy an architecture-specific static QEMU binary into the image. The resulting Dockerfile is built into an image called `weave:${VERSION}` if the target architecture is amd64, or `weave${ARCHITECTURE}:${VERSION}` if not.

The other templates create Dockerfiles that select either the `weave:${VERSION}` or the `weave${ARCHITECTURE}:${VERSION}` image as the base, depending on the target architecture.

Next, the generated Dockerfiles are used to build images. The images are tagged with several labels. One of these labels is `org.opencontainers.image.revision`, whose value is supposed to be a Git commit hash from which the image was built. It is passed in through a Dockerfile argument called `revision`, and can be supplied in the overall build process through the Makefile parameter `GIT_REVISION`. By default, this is picked up by running ` git rev-parse HEAD` in the current directory.

Once built, the images are tagged as `${ACCOUNTNAME}/${IMAGENAME}:latest` if the target architecture is amd64, or `${ACCOUNTNAME/${IMAGENAME}${ARCHITECTURE}:latest` if the target architecture is anything else. The account name in the tag can be controlled using the Makefile parameter `DOCKERHUB_USER`.

The Makefile includes rules to push these images, which involve pushing all architecture-specific images, and then pushing a multi-architecture manifest.

The Makefile also includes rules to build a Docker plugin, and to run tests.

## Important Makefile parameters

|Name|Description|Default|
|---|---|---|
|ARCH|The build target architecture. Can be one of: `amd64` `arm` `arm64` `ppc64le` `s390x`|`amd64`|
|BUILD_IN_CONTAINER|`true` if the build is to be done in a container created from a build container image, `false` if it is to be done on the local machine.|`true`|
|ALPINE_BASEIMAGE|The base image for all generated images.|`alpine:3.10`|
|GIT_REVISION|The git commit hash used as the value for a label called `org.opencontainers.image.revision` in all generated images.|Result of `git rev-parse HEAD`|
|DOCKERHUB_USER|The registry/account used to tag all generated images|`weaveworks`|
