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

# Set scan directory
SCANDIR="${BINDIR}/../scans"
mkdir -p "${SCANDIR}"

# Scan images
grype "${WEAVE_KUBE_IMAGE}" --add-cpes-if-none >"${SCANDIR}/weave-kube-list-vulns.txt"
grype "${WEAVE_NPC_IMAGE}" --add-cpes-if-none >"${SCANDIR}/weave-npc-list-vulns.txt"

UNIQUECOUNT=$(tail -n +2 -q "${SCANDIR}/weave-npc-list-vulns.txt" "${SCANDIR}/weave-kube-list-vulns.txt" | sort -u | wc -l)
BADGECOLOR="blue"

if [ "$UNIQUECOUNT" -gt "0" ]; then
    BADGECOLOR="orange"
fi

# Produce report
printf "# Vulnerability Report\n\n" >  "${SCANDIR}/report.md"
{
    printf "Report date: %s\n" "$(date +'%Y-%m-%d')"
    printf "Unique vulnerability count: %s\n" "${UNIQUECOUNT}" 
    #tail -n +2 -q "${SCANDIR}/weave-npc-list-vulns.txt" "${SCANDIR}/weave-kube-list-vulns.txt" | sort -u | wc -l
    printf "Images version: %s\n" "${IMAGE_VERSION}"
    printf "\n## Scanner Details\n\n"
    grype version
    printf "\n## Vulnerabilities\n\nweave-kube: (%s) \n\n" "$(tail +2 "${SCANDIR}/weave-kube-list-vulns.txt" | wc -l)"
    cat "${SCANDIR}/weave-kube-list-vulns.txt"
    printf "\nweave-npc: (%s)\n\n" "$(tail +2 "${SCANDIR}/weave-npc-list-vulns.txt" | wc -l)"
    cat "${SCANDIR}/weave-npc-list-vulns.txt"
} >> "${SCANDIR}/report.md"

# Produce Vulnerability Count badge json for README
cat <<EOBADGE > "${SCANDIR}/badge.json"
{"schemaVersion": 1, "label": "Vulnerabilty count", "message": "${UNIQUECOUNT}", "color": "${BADGECOLOR}"}
EOBADGE