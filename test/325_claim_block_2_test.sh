#! /bin/bash

. ./config.sh

UNIVERSE=10.32.0.0/12

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

start_suite "Block while claiming addresses"

weave_on $HOST1 launch --ipalloc-range $UNIVERSE $HOST2
weave_on $HOST2 launch --ipalloc-range $UNIVERSE $HOST2

start_container $HOST1 --name c1
C1=$(container_ip $HOST1 c1)
docker_on $HOST1 stop c1

stop_weave_on $HOST1
stop_weave_on $HOST2
# Do not remove the state of host2 to keep the previously established ring.
delete_persistence $HOST1

weave_on $HOST1 launch --ipalloc-range $UNIVERSE $HOST2

# Start another container on host1. The starting should block, because host1 is
# not able to establish the ring due to host2 being offline.
CMD="proxy_start_container $HOST1 --name c2 -e WEAVE_CIDR=$C1/12"
assert_raises "timeout 5 cat <( $CMD )" 124

# However, allocation for an external subnet should not block.
assert_raises "proxy_start_container $HOST1 -e WEAVE_CIDR=10.48.0.1/12"

# Launch host2, so that host1 can establish the ring.
weave_on $HOST2 launch --ipalloc-range $UNIVERSE $HOST1
wait_for_container_ip $HOST1 c2
assert_raises "[ $(container_ip $HOST1 c2) == $C1 ]"

end_suite
