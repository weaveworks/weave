#!/bin/bash

set -e

# Signal failures in lock file, in order to fail fast:
function signal_failure() {
    echo "KO" >"$TEST_VMS_PROV_AND_CONF_LOCK_FILE"
    exit 1
}
trap signal_failure ERR

function install_terraform() {
    curl -fsS https://releases.hashicorp.com/terraform/0.11.14/terraform_0.11.14_linux_amd64.zip | gunzip >terraform && chmod +x terraform && sudo mv terraform /usr/bin
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
