#! /bin/bash
# Explicitly not called _test.sh - this isn't run, but imported by other tests.

. ./600_proxy_docker_py.sh

docker_py_test 2 4
