#!/bin/bash

. ./config.sh

start_suite "Run weave with --local"

run_on $HOST1 sudo ./weave --local reset

run_on $HOST1 sudo ./weave --local launch
assert_raises "docker_on $HOST1 ps | grep weave"

run_on $HOST1 sudo ./weave --local run 10.2.6.5/24 -d -ti --name=c1 gliderlabs/alpine /bin/sh
assert_raises "exec_on $HOST1 c1 ifconfig | grep ethwe"

end_suite
