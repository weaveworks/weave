#! /bin/bash

. ./config.sh

UNIVERSE=10.32.0.0/12
C1=10.40.0.1

delete_persistence() {
    for host in "$@" ; do
        docker_on $host rm -v weavedb >/dev/null
        docker_on $host rm weave >/dev/null
    done
}

wait_for_container_ip() {
    host=$1
    container=$2
    for i in $(seq 1 120); do
        echo "Waiting for $container ip at $host"
        if container_ip $host $container > /dev/null 2>&1 ; then
            return
        fi
        sleep 1
    done
    echo "Timed out waiting for $container ip at $host" >&2
    exit 1
}

start_suite "Claiming addresses"

weave_on $HOST1 launch-router --ipalloc-range $UNIVERSE $HOST2
weave_on $HOST2 launch-router --ipalloc-range $UNIVERSE $HOST1

start_container $HOST1 $C1/12 --name=c1
start_container $HOST2        --name=c2
assert_raises "exec_on $HOST2 c2 $PING $C1"

stop_weave_on $HOST1
stop_weave_on $HOST2

# Delete persistence data so they form a blank ring
delete_persistence $HOST1 $HOST2

# Start hosts in reverse order so c1's address has to be claimed from host2
weave_on $HOST2 launch-router --ipalloc-range $UNIVERSE
weave_on $HOST1 launch-router --ipalloc-range $UNIVERSE $HOST2

# Start another container on host2, so if it hasn't relinquished c1's
# address it would give that out as the first available.
start_container $HOST2 --name=c3
C3=$(container_ip $HOST2 c3)
assert_raises "[ $C3 != $C1 ]"

sleep 1 # give routers some time to fully establish connectivity
assert_raises "exec_on $HOST1 c1 $PING $C3"

stop_weave_on $HOST1
stop_weave_on $HOST2

delete_persistence $HOST1 $HOST2

# Now make host1 attempt to claim from host2, when host2 is stopped
# the point being to check whether host1 will hang trying to talk to host2
weave_on $HOST2 launch-router --ipalloc-range $UNIVERSE
# Introduce host3 to remember the IPAM CRDT when we stop host2
weave_on $HOST3 launch-router --ipalloc-range $UNIVERSE $HOST2
weave_on $HOST3 prime
stop_weave_on $HOST2
weave_on $HOST1 launch-router --ipalloc-range $UNIVERSE $HOST3

stop_weave_on $HOST1
stop_weave_on $HOST3
delete_persistence $HOST1 $HOST2 $HOST3

weave_on $HOST1 launch --ipalloc-range $UNIVERSE $HOST2
weave_on $HOST2 launch --ipalloc-range $UNIVERSE $HOST1

start_container $HOST1 --name c4
C4=$(container_ip $HOST1 c4)
docker_on $HOST1 stop c4

stop_weave_on $HOST1
stop_weave_on $HOST2
# Do not remove the state of host2 to keep the previously established ring.
delete_persistence $HOST1

weave_on $HOST1 launch --ipalloc-range $UNIVERSE $HOST2

# Start another container on host1. The starting should block, because host1 is
# not able to establish the ring due to host2 being offline.
CMD="proxy_start_container $HOST1 --name c5 -e WEAVE_CIDR=$C4/12"
assert_raises "timeout 5 cat <( $CMD )" 124

# However, allocation for an external subnet should not block.
assert_raises "proxy_start_container $HOST1 -e WEAVE_CIDR=10.48.0.1/12"

# Launch host2, so that host1 can establish the ring.
weave_on $HOST2 launch --ipalloc-range $UNIVERSE $HOST1
wait_for_container_ip $HOST1 c5
assert_raises "[ $(container_ip $HOST1 c5) == $C4 ]"

end_suite
