#! /bin/bash

. ./config.sh

start_suite "Specify initial IPAM seed"

# Launch two disconnected routers
weave_on $HOST1 launch --name 00:00:00:00:00:01 --ipam-seed 00:00:00:00:00:01,00:00:00:00:00:02
weave_on $HOST2 launch --name 00:00:00:00:00:02 --ipam-seed 00:00:00:00:00:01,00:00:00:00:00:02

# Ensure allocations can proceed
assert_raises "timeout 10 cat <( start_container $HOST1 --name c1)"
assert_raises "timeout 10 cat <( start_container $HOST2 --name c2)"

# Connect routers
weave_on $HOST2 connect $HOST1

# Check connectivity
assert_raises "exec_on $HOST1 c1 $PING c2"
assert_raises "exec_on $HOST2 c2 $PING c1"

end_suite
