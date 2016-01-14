#!/bin/bash

set -e

. ./config.sh

export PROJECT=positive-cocoa-90213
export TEMPLATE_NAME="test-template-9"
export NUM_HOSTS=5
. "../tools/integration/gce.sh" "$@"
