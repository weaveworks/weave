#!/bin/sh
set -e

# Get directory of script file
a="/$0"; a="${a%/*}"; a="${a:-.}"; a="${a##/}/"; BINDIR=$(cd "$a"; pwd)

rm  "${BINDIR}/../scans/"*