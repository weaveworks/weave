#!/bin/bash

set -e

. ./config.sh

../tools/integration/run_all.sh "$@"
