#! /bin/bash

. ./config.sh

start_suite "Checking persistence of IPAM"

launch_router_with_db() {
    host=$1
    shift
    WEAVE_DOCKER_ARGS="-v /tmp:/db" weave_on $host launch-router --db-prefix=/db/test162- "$@"
}

# Remove any persisted data from previous runs
run_on $HOST1 "sudo rm -f /tmp/test162-*"
run_on $HOST2 "sudo rm -f /tmp/test162-*"

launch_router_with_db $HOST1 $HOST2
launch_router_with_db $HOST2 $HOST1

start_container $HOST1 --name=c1
C1=$(container_ip $HOST1 c1)
start_container $HOST2 --name=c2
assert_raises "exec_on $HOST2 c2 $PING $C1"

stop_router_on $HOST1
stop_router_on $HOST2

# Start just HOST2; if nothing persisted it would form its own ring
launch_router_with_db $HOST2
start_container $HOST2 --name=c3
C3=$(container_ip $HOST2 c3)
assert_raises "[ $C3 != $C1 ]"

# Restart HOST1 and see if it remembers to connect to HOST2
launch_router_with_db $HOST1
assert_raises "exec_on $HOST2 c2 $PING $C1"

end_suite
