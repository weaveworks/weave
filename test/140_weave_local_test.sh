#!/bin/bash

. ./config.sh

start_suite "Run weave with --local"

run_on $HOST1 sudo COVERAGE=$COVERAGE weave --local reset

run_on $HOST1 sudo COVERAGE=$COVERAGE weave --local launch --ipalloc-range 10.2.5.0/24
assert_raises "docker_on $HOST1 ps | grep weave"

run_on $HOST1 sudo COVERAGE=$COVERAGE weave --local run 10.2.6.5/24 -ti --name=c1 $SMALL_IMAGE /bin/sh
assert_raises "exec_on $HOST1 c1 $CHECK_ETHWE_UP"

run_on $HOST1 sudo COVERAGE=$COVERAGE weave --local run             -ti --name=c2 $SMALL_IMAGE /bin/sh
assert_raises "exec_on $HOST1 c2 $CHECK_ETHWE_UP"

end_suite
