#!/bin/bash

. "$(dirname "$0")/config.sh"

weave_local_on() {
    host=$1
    shift 1
    run_on $host sudo COVERAGE=$COVERAGE weave --local $@
}

start_suite "Run weave with --local"

weave_local_on $HOST1 reset

weave_local_on $HOST1 launch --ipalloc-range 10.2.5.0/24
assert_raises "docker_on $HOST1 ps | grep weave"

weave_local_on $HOST1 run 10.2.6.5/24 -ti --name=c1 $SMALL_IMAGE /bin/sh
wait_for_attached $HOST1 c1

weave_local_on $HOST1 run             -ti --name=c2 $SMALL_IMAGE /bin/sh
wait_for_attached $HOST1 c2

assert "weave_local_on $HOST1 ps | wc -l" 3

end_suite
