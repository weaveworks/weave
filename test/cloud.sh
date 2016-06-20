#!/bin/bash

CLOUD_EXEC="./gce.sh"
if [ "$CLOUD_ENV" == "aws" ]; then
    CLOUD_EXEC="./aws.sh"
fi

$CLOUD_EXEC "$@"
