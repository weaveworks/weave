#! /bin/bash

. ./config.sh

C1=10.2.1.4
C2=10.2.1.7

start_suite "Connecting and forgetting routers after launch"

weave_on $HOST1 launch
weave_on $HOST2 launch

start_container $HOST1 $C1/24 --name=c1
start_container $HOST2 $C2/24 --name=c2

assert_raises "exec_on $HOST1 c1 $PING $C2" 1
assert_raises "weave_on $HOST2 connect $HOST1 $HOST2"
assert_raises "exec_on $HOST1 c1 $PING $C2"

assert_raises "weave_on $HOST2 forget $HOST1 $HOST2"
assert_raises "weave_on $HOST1 stop"
assert_raises "weave_on $HOST1 launch"

assert_raises "exec_on $HOST1 c1 $PING $C2" 1

end_suite
