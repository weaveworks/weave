#! /bin/sh
set -e

# These variables are used to tag the images. Change as 
# required.
: "${IMAGE_VERSION:=}"
: "${REGISTRY_USER:=}"

# This variable is used to flag plugin publishing
: "${PUBLISH:=}"

if [ -z "${IMAGE_VERSION}" ] || [ -z "${REGISTRY_USER}" ] ; then
    >&2 echo "Please provide valid values for IMAGE_VERSION and REGISTRY_USER." 
    exit 1
fi

WEAVER_IMAGE=${REGISTRY_USER}/weave:${IMAGE_VERSION}
PLUGIN_IMAGE=${REGISTRY_USER}/net-plugin:${IMAGE_VERSION}
PLUGIN_BUILD_IMG="plugin-builder"
PLUGIN_PARENT_DIR="../prog/net-plugin"
PLUGIN_WORK_DIR="${PLUGIN_PARENT_DIR}/rootfs"

docker container rm -f ${PLUGIN_BUILD_IMG} 2>/dev/null
docker container create --name=${PLUGIN_BUILD_IMG} "${WEAVER_IMAGE}" true
rm -rf ${PLUGIN_WORK_DIR}
mkdir ${PLUGIN_WORK_DIR}
docker export ${PLUGIN_BUILD_IMG} | tar -x -C ${PLUGIN_WORK_DIR}
docker container rm -f ${PLUGIN_BUILD_IMG}
cp "${PLUGIN_PARENT_DIR}/launch.sh" "${PLUGIN_WORK_DIR}/home/weave/launch.sh"
set +e
docker plugin disable "${PLUGIN_IMAGE}" 2>/dev/null
docker plugin rm "${PLUGIN_IMAGE}" 2>/dev/null
set -e
docker plugin create "${PLUGIN_IMAGE}" "${PLUGIN_PARENT_DIR}"

if [ "${PUBLISH}" = "true" ]; then
    docker plugin push "${PLUGIN_IMAGE}"
fi