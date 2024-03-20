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
: "${ALPINE_BASEIMAGE:=alpine:3.19.1}"
: "${WEAVE_VERSION=${IMAGE_VERSION}}"
: "${GIT_REVISION=$(git rev-parse HEAD)}"
: "${PLATFORMS:=linux/amd64,linux/arm,linux/arm64,linux/ppc64le,linux/s390x}"
: "${PUBLISH:=}"

if [ "$PUBLISH" = "true" ]; then
    POSTBUILD="--push"
elif [ "$PUBLISH" = "false" ]; then
    POSTBUILD="--load"
else
    POSTBUILD=""
fi

# These are the names of the images
WEAVER_IMAGE=${REGISTRY_USER}/weave
WEAVEEXEC_IMAGE=${REGISTRY_USER}/weaveexec
WEAVEKUBE_IMAGE=${REGISTRY_USER}/weave-kube
WEAVENPC_IMAGE=${REGISTRY_USER}/weave-npc
WEAVEDB_IMAGE=${REGISTRY_USER}/weavedb
NETWORKTESTER_IMAGE=${REGISTRY_USER}/network-tester

build_image() {
    IMAGENAME=$2
    IMAGETAG=${IMAGENAME}:${IMAGE_VERSION}
    if [ "$PUBLISH" = "true" ]; then
        # When an image is published to a registry, also tag it
        # with ':latest', unless the image version string 
        # contains '-beta'
        case "$IMAGE_VERSION" in
            *-beta*) IS_BETA=1 ;;
            *) IS_BETA= ;;
        esac
        [ -z "${IS_BETA}" ] && PUBLISHTAGOPT="-t ${IMAGENAME}:latest"
    else
        PUBLISHTAGOPT=""
    fi

    # Get directory of script file
    a="/$0"; a="${a%/*}"; a="${a:-.}"; a="${a##/}/"; BINDIR=$(cd "$a"; pwd)
    
    cd "$BINDIR/../.."

    # shellcheck disable=SC2086
    docker buildx build \
            ${POSTBUILD} \
            --progress=plain \
            --platform=${PLATFORMS} \
            --target="$1" \
            --build-arg=ALPINE_BASEIMAGE=${ALPINE_BASEIMAGE} \
            --build-arg=WEAVE_VERSION=${WEAVE_VERSION} \
            --build-arg=revision=${GIT_REVISION} \
            --build-arg=imageversion=${IMAGE_VERSION} \
            -f reweave/build/Dockerfile \
            -t "${IMAGETAG}" \
            ${PUBLISHTAGOPT} \
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