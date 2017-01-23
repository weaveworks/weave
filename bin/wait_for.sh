#!/bin/bash

wait_for() {
    local timeout=$1
    local lock_file=$2
    for i in $(seq 1 "$timeout"); do
        [ -f "$lock_file" ] && echo "[$i seconds]: $lock_file found." && return
        if ! ((i % 10)); then echo "[$i seconds]: Waiting for $lock_file to be created..."; fi
        sleep 1
    done
    echo "Timed out waiting for test VMs to be ready. See details in: $TEST_VMS_SETUP_OUTPUT_FILE" >&2
    exit 1
}
