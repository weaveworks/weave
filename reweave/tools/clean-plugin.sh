#! /bin/sh
set -e

# These variables are used to tag the images. Change as 
# required.
: "${IMAGE_VERSION:=}"
: "${REGISTRY_USER:=}"

if [ -z "${IMAGE_VERSION}" ] || [ -z "${REGISTRY_USER}" ] ; then
    >&2 echo "Please provide valid values for IMAGE_VERSION and REGISTRY_USER." 
    exit 1
fi

PLUGIN_IMAGE=${REGISTRY_USER}/net-plugin:${IMAGE_VERSION}

docker plugin rm "${PLUGIN_IMAGE}" 2>/dev/null