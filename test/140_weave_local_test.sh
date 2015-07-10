#!/bin/bash

. ./config.sh

start_suite "Run weave with --local"

weave_local_on() {
    host=$1
    shift 1
    run_on $host sudo COVERAGE=$COVERAGE weave --local $@
}

weave_local_on $HOST1 reset

weave_local_on $HOST1 launch --ipalloc-range 10.2.5.0/24
assert_raises "docker_on $HOST1 ps | grep weave"

weave_local_on $HOST1 run 10.2.6.5/24 -ti --name=c1 $SMALL_IMAGE /bin/sh
assert_raises "exec_on $HOST1 c1 $CHECK_ETHWE_UP"

weave_local_on $HOST1 run             -ti --name=c2 $SMALL_IMAGE /bin/sh
assert_raises "exec_on $HOST1 c2 $CHECK_ETHWE_UP"

end_suite
