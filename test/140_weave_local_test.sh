#!/bin/bash

. ./config.sh

start_suite "Run weave with --local"

run_on $HOST1 sudo ./weave --local reset

run_on $HOST1 sudo ./weave --local launch -iprange 10.2.5.0/24 >/dev/null
assert_raises "docker_on $HOST1 ps | grep weave"

run_on $HOST1 sudo ./weave --local run 10.2.6.5/24 -ti --name=c1 gliderlabs/alpine /bin/sh >/dev/null
assert_raises "exec_on $HOST1 c1 ip link show ethwe"

run_on $HOST1 sudo ./weave --local run -ti --name=c2 gliderlabs/alpine /bin/sh >/dev/null
assert_raises "exec_on $HOST1 c2 ifconfig | grep ethwe"

end_suite
