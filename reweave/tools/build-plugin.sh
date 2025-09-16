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

# To build the plugin for multiple architecture, the 
# local Docker engine needs to have the containerd
# snapshotter enabled.
if [ "overlayfs" != "$(docker info -f '{{.Driver}}')" ]; then
    echo "To ensure that the plugin can be built for multiple architectures, the"
    echo "Docker Engine needs to have the containerd image store feature enabled."
    echo "Please see https://docs.docker.com/storage/containerd/ to find out how."
    echo "Run this build again after enabling it."
    exit 1
fi


# This variable is used to control the architectures for
# which the plugin will be built.
: "${PLATFORMS:=amd64,arm,arm64,ppc64le,s390x}"

# This variable is used to flag plugin publishing
: "${PUBLISH:=}"

WEAVER_IMAGE=${REGISTRY_USER}/weave:${IMAGE_VERSION}

# $1 - Architecture to build for
build_plugin() {
    PLUGIN_ARCH=${1:-amd64}
    PLUGIN_IMAGE=${REGISTRY_USER}/net-plugin:${IMAGE_VERSION}-${PLUGIN_ARCH}
    PLUGIN_BUILD_IMG="plugin-builder-${PLUGIN_ARCH}"
    PLUGIN_PARENT_DIR="../prog/net-plugin"
    PLUGIN_WORK_DIR="${PLUGIN_PARENT_DIR}/rootfs"

    echo "Building ${PLUGIN_IMAGE}..."

    docker container rm -f "${PLUGIN_BUILD_IMG}" 2>/dev/null
    docker container create --platform="${PLUGIN_ARCH}" --name="${PLUGIN_BUILD_IMG}" "${WEAVER_IMAGE}" true
    rm -rf ${PLUGIN_WORK_DIR}
    mkdir ${PLUGIN_WORK_DIR}
    docker export "${PLUGIN_BUILD_IMG}" | tar -x -C ${PLUGIN_WORK_DIR}
    docker container rm -f "${PLUGIN_BUILD_IMG}"
    cp "${PLUGIN_PARENT_DIR}/launch.sh" "${PLUGIN_WORK_DIR}/home/weave/launch.sh"
    set +e
    docker plugin disable "${PLUGIN_IMAGE}" 2>/dev/null
    docker plugin rm "${PLUGIN_IMAGE}" 2>/dev/null
    set -e
    docker plugin create "${PLUGIN_IMAGE}" "${PLUGIN_PARENT_DIR}"

    if [ "${PUBLISH}" = "true" ]; then
        echo "Pushing ${PLUGIN_IMAGE}..."
        docker plugin push "${PLUGIN_IMAGE}"

        # If this is not a beta release, also create a "latest-release-$ARCH"
        # plugin.
        case "$IMAGE_VERSION" in
            *-beta*) IS_BETA=1 ;;
            *) IS_BETA= ;;
        esac

        if [ -z ${IS_BETA} ]; then
            docker plugin rm "${PLUGIN_IMAGE}" 2>/dev/null
            echo "Tagging ${PLUGIN_IMAGE} as latest_release..."
            LATEST_IMAGE=${REGISTRY_USER}/net-plugin:latest_release-${PLUGIN_ARCH}
            docker plugin create "${LATEST_IMAGE}" "${PLUGIN_PARENT_DIR}"
            echo "Pushing ${LATEST_IMAGE}..."
            docker plugin push "${LATEST_IMAGE}"
        fi

    fi

    echo "Done."
}

for i in $( echo "$PLATFORMS" | tr ',' ' '); do
    build_plugin "$i"
done
