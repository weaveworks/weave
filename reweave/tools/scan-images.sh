#!/bin/sh
set -e

# These variables are used to choose the images to scan. Change as 
# required.
: "${IMAGE_VERSION:=}"
: "${REGISTRY_USER:=}"

if [ -z "${IMAGE_VERSION}" ] || [ -z "${REGISTRY_USER}" ] ; then
    >&2 echo "Please provide valid values for IMAGE_VERSION and REGISTRY_USER." 
    exit 1
fi

echo "Scanning images and collecting data..."

# Currently, we are interested only in the weave-kube and weave-npc
# images. 
WEAVE_KUBE_IMAGE="${REGISTRY_USER}/weave-kube:${IMAGE_VERSION}"
WEAVE_NPC_IMAGE="${REGISTRY_USER}/weave-npc:${IMAGE_VERSION}"

# Get directory of script file
a="/$0"; a="${a%/*}"; a="${a:-.}"; a="${a##/}/"; BINDIR=$(cd "$a"; pwd)
mkdir -p "${BINDIR}/../scans"

echo "Image version: ${IMAGE_VERSION}" >"${BINDIR}/../scans/images-version.txt"

grype version >"${BINDIR}/../scans/scanner-version.txt"

grype "${WEAVE_KUBE_IMAGE}" --add-cpes-if-none >"${BINDIR}/../scans/weave-kube-list-vulns.txt"
grype "${WEAVE_NPC_IMAGE}" --add-cpes-if-none >"${BINDIR}/../scans/weave-npc-list-vulns.txt"

echo "weave-kube CVE Count: $(tail +3 "${BINDIR}/../scans/weave-kube-list-vulns.txt" | wc -l)" \
    >"${BINDIR}/../scans/weave-kube-count-vulns.txt"

echo "weave-npc CVE Count: $(tail +3 "${BINDIR}/../scans/weave-npc-list-vulns.txt" | wc -l)" \
    >"${BINDIR}/../scans/weave-npc-count-vulns.txt"
