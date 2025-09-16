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

# These are the names of the images
WEAVER_IMAGE=${REGISTRY_USER}/weave:${IMAGE_VERSION}
WEAVEEXEC_IMAGE=${REGISTRY_USER}/weaveexec:${IMAGE_VERSION}
WEAVEKUBE_IMAGE=${REGISTRY_USER}/weave-kube:${IMAGE_VERSION}
WEAVENPC_IMAGE=${REGISTRY_USER}/weave-npc:${IMAGE_VERSION}
WEAVEDB_IMAGE=${REGISTRY_USER}/weavedb:${IMAGE_VERSION}
NETWORKTESTER_IMAGE=${REGISTRY_USER}/network-tester:${IMAGE_VERSION}

docker image rm "${WEAVER_IMAGE}" "${WEAVEEXEC_IMAGE}" "${WEAVEKUBE_IMAGE}" \
                "${WEAVENPC_IMAGE}" "${WEAVEDB_IMAGE}" "${NETWORKTESTER_IMAGE}"

