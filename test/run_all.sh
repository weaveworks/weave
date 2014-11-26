#!/bin/bash

. ./config.sh

whitely echo Sanity checks
if ! bash ./sanity_check.sh; then
    whitely echo ...failed
    exit 1
fi
whitely echo ...ok

for t in *_test.sh; do
    echo
    greyly echo "---= Running $t =---"
    . $t
done
