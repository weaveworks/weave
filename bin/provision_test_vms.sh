#!/bin/bash

set -eo pipefail

# Signal failures in lock file, in order to fail fast:
function signal_failure() {
    echo "KO" >"$TEST_VMS_PROV_AND_CONF_LOCK_FILE"
    exit 1
}
trap signal_failure ERR


function install_terraform() {
    TF_VERSION="0.12.0"
    TF_SHA256SUM="42ffd2db97853d5249621d071f4babeed8f5fdba40e3685e6c1013b9b7b25830"
    curl -fsS -o terraform.zip "https://releases.hashicorp.com/terraform/${TF_VERSION}/terraform_${TF_VERSION}_linux_amd64.zip"
    echo "${TF_SHA256SUM} terraform.zip" | sha256sum -c
    sudo unzip terraform.zip -d /usr/bin
}

[ -n "$SECRET_KEY" ] || {
    echo "Cannot run smoke tests: no secret key"
    exit 1
}

source "$SRCDIR/bin/circle-env"

install_terraform

# Only attempt to create GCP image in first container, wait for it to be created otherwise:
[ "$CIRCLE_NODE_INDEX" != "0" ] && export CREATE_IMAGE=0

# Provision and configure testing VMs:
cd "$SRCDIR/test" # Ensures we generate Terraform state files in the right folder, for later use by integration tests.
./run-integration-tests.sh configure
echo "OK" >"$TEST_VMS_PROV_AND_CONF_LOCK_FILE"
echo "Test VMs now provisioned and configured. $(date)."
