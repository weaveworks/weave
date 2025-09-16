#!/bin/sh
set -e

# These variables are used to choose the images to scan. Change as 
# required.
: "${IMAGE_VERSION:=}"
: "${REGISTRY_USER:=}"
: "${IMAGE_LIST:=weave-kube weave-npc weave weaveexec}"

if [ -z "${IMAGE_VERSION}" ] || [ -z "${REGISTRY_USER}" ] ; then
    >&2 echo "Please provide valid values for IMAGE_VERSION and REGISTRY_USER." 
    exit 1
fi

echo "Scanning images and collecting data..."

# Get directory of script file
a="/$0"; a="${a%/*}"; a="${a:-.}"; a="${a##/}/"; BINDIR=$(cd "$a"; pwd)

# Set scan directory
SCANDIR="${BINDIR}/../scans"
mkdir -p "${SCANDIR}"

# Scan images
for im in ${IMAGE_LIST};do
    grype "${REGISTRY_USER}/${im}:${IMAGE_VERSION}" --add-cpes-if-none >"${SCANDIR}/${im}-list-vulns.txt"
done

#UNIQUECOUNT=$(tail -n +2 -q "${SCANDIR}/weave-npc-list-vulns.txt" "${SCANDIR}/weave-kube-list-vulns.txt" | sort -u | wc -l)
UNIQUECOUNT=$(tail -n +2 -q "${SCANDIR}"/*-list-vulns.txt  | sed -e "s/\s\{2,\}/,/g" | sort -u | wc -l)
BADGECOLOR="blue"

if [ "$UNIQUECOUNT" -gt "0" ]; then
    BADGECOLOR="orange"
fi

echo "Generating report..."

# Produce report
printf "# Vulnerability Report\n\n" >  "${SCANDIR}/report.md"
{
    printf "\`\`\`\n"
    printf "Report date: %s\n" "$(date +'%Y-%m-%d')"
    printf "Unique vulnerability count: %s\n" "${UNIQUECOUNT}" 
    printf "Images version: %s\n" "${IMAGE_VERSION}"
    printf "\`\`\`\n"
    printf "\n## Scanner Details\n\n"
    printf "\`\`\`\n"
    grype version
    printf "\`\`\`\n"
    printf "\n## Vulnerabilities\n\n"
    for im in ${IMAGE_LIST};do
        printf "### ${im}: (%s) \n\n" "$(tail +2 "${SCANDIR}/${im}-list-vulns.txt" | wc -l)"
        printf "\`\`\`\n"
        cat "${SCANDIR}/${im}-list-vulns.txt"
        printf "\`\`\`\n\n"
    done

} >> "${SCANDIR}/report.md"

rm -f "${SCANDIR}"/*-list-vulns.txt

echo "Generating badge..."

# Produce Vulnerability Count badge json for README
cat <<EOBADGE > "${SCANDIR}/badge.json"
{"schemaVersion": 1, "label": "Vulnerabilty count", "message": "${UNIQUECOUNT}", "color": "${BADGECOLOR}"}
EOBADGE

echo "Done."