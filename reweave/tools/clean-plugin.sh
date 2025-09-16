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

# This variable is used to control the architectures for
# which the plugin will be built.
: "${PLATFORMS:=amd64,arm,arm64,ppc64le,s390x}"

# $1 - Architecture to build for
delete_plugin() {
    PLUGIN_ARCH=${1:-amd64}
    PLUGIN_IMAGE=${REGISTRY_USER}/net-plugin:${IMAGE_VERSION}-${PLUGIN_ARCH}
    echo "Deleting plugin ${PLUGIN_IMAGE}..."
    set +e
    docker plugin rm "${PLUGIN_IMAGE}" 2>/dev/null
    set -e
}

for i in $( echo "$PLATFORMS" | tr ',' ' '); do
    delete_plugin "$i"
done

rm -rf "../prog/net-plugin/rootfs"