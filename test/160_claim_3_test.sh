#! /bin/bash

. ./config.sh

start_suite "Claiming addresses"

weave_on $HOST1 launch-router
weave_on $HOST2 launch-router $HOST1

start_container $HOST1 --name=c1
start_container $HOST2 --name=c2
C1=$(container_ip $HOST1 c1)
assert_raises "exec_on $HOST2 c2 $PING $C1"

stop_router_on $HOST1
stop_router_on $HOST2

# Start hosts in reverse order so c1's address has to be claimed from host2
weave_on $HOST2 launch-router
weave_on $HOST1 launch-router $HOST2

# Start another container on host2, so if it hasn't relinquished c1's
# address it would give that out as the first available.
start_container $HOST2 --name=c3
C3=$(container_ip $HOST2 c3)
assert_raises "[ $C3 != $C1 ]"

sleep 1 # give routers some time to fully establish connectivity
assert_raises "exec_on $HOST1 c1 $PING $C3"

stop_router_on $HOST1
stop_router_on $HOST2

# Now make host1 attempt to claim from host2, when host2 is stopped
weave_on $HOST2 launch-router
# Introduce host3 to remember the IPAM CRDT when we stop host2
weave_on $HOST3 launch-router $HOST2
stop_router_on $HOST2
weave_on $HOST1 launch-router $HOST3

end_suite
