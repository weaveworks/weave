#! /bin/bash

. ./config.sh

start_suite "Check restart re-uses same container when possible"

ID=$(weave_on $HOST1 launch-router)
weave_on $HOST1 stop-router
assert "weave_on $HOST1 launch-router" "$ID"
# Can't start if already running with different args
assert_raises "weave_on $HOST1 launch-router --log-level=debug" 1
# Stop then start with different arg
weave_on $HOST1 stop-router
ID2=$(weave_on $HOST1 launch-router --log-level=debug)
assert "[ $ID != $ID2 ]"

# Stop then start with different password
weave_on $HOST1 stop-router
ID3=$(weave_on $HOST1 launch-router --password=xyzzy)
assert "[ $ID2 != $ID3 ]"
# Stop then start with same password
weave_on $HOST1 stop-router
assert "weave_on $HOST1 launch-router --password=xyzzy" "$ID3"

# Fails on bad argument
assert_raises "weave_on $HOST1 launch-plugin --foo" 1
# Fails again on bad argument, although there is a dead container
assert_raises "weave_on $HOST1 launch-plugin --foo" 1

end_suite
