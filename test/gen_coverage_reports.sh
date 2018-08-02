#!/bin/bash

set -ex
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# 'cover' tool is from github.com/weaveworks/build-tools/cover

cover ./coverage/* >profile.cov
go tool cover -html=profile.cov -o coverage.html
go tool cover -func=profile.cov -o coverage.txt
tar czf coverage.tar.gz ./coverage
