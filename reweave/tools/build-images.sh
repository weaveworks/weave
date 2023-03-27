#!/bin/sh
set -e

# These variables are used to tag the images. Change as 
# required.
: "${IMAGE_VERSION:=}"
: "${REGISTRY_USER:=}"

if [ -z "${IMAGE_VERSION}" ] || [ -z "${REGISTRY_USER}" ] ; then
    >&2 echo "Please provide valid values for IMAGE_VERSION and REGISTRY_USER." 
    exit 1
fi

# These variables are used to control the build process
# Change with care.
: "${ALPINE_BASEIMAGE:=alpine:3.17.2}"
: "${WEAVE_VERSION=git-$(git rev-parse --short=12 HEAD)}"
: "${GIT_REVISION=$(git rev-parse HEAD)}"
: "${PLATFORMS:=linux/amd64,linux/arm,linux/arm64,linux/ppc64le,linux/s390x}"
: "${POSTBUILD:=}"

# These are the names of the images
WEAVER_IMAGE=${REGISTRY_USER}/weave:${IMAGE_VERSION}
WEAVEEXEC_IMAGE=${REGISTRY_USER}/weaveexec:${IMAGE_VERSION}
WEAVEKUBE_IMAGE=${REGISTRY_USER}/weave-kube:${IMAGE_VERSION}
WEAVENPC_IMAGE=${REGISTRY_USER}/weave-npc:${IMAGE_VERSION}
WEAVEDB_IMAGE=${REGISTRY_USER}/weavedb:${IMAGE_VERSION}
NETWORKTESTER_IMAGE=${REGISTRY_USER}/network-tester:${IMAGE_VERSION}

build_image() {
    # Get directory of script file
    a="/$0"; a="${a%/*}"; a="${a:-.}"; a="${a##/}/"; BINDIR=$(cd "$a"; pwd)
    
    cd "$BINDIR/../.."

    # shellcheck disable=SC2086
    docker buildx build \
            ${POSTBUILD} \
            --platform=${PLATFORMS} \
            --target="$1" \
            --build-arg=ALPINE_BASEIMAGE=${ALPINE_BASEIMAGE} \
            --build-arg=WEAVE_VERSION=${WEAVE_VERSION} \
            --build-arg=revision=${GIT_REVISION} \
            --build-arg=imageversion=${IMAGE_VERSION} \
            -f reweave/build/Dockerfile \
            -t "$2" \
            .

    cd -
}

# shellcheck disable=SC2086
{
build_image "weaverimage" ${WEAVER_IMAGE}
build_image "weavexecimage" ${WEAVEEXEC_IMAGE}
build_image "weavekubeimage" ${WEAVEKUBE_IMAGE}
build_image "weavenpcimage" ${WEAVENPC_IMAGE}
build_image "weavedbimage" ${WEAVEDB_IMAGE}
build_image "networktesterimage" ${NETWORKTESTER_IMAGE}
}