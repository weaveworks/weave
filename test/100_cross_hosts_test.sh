#! /bin/bash

. ./config.sh

C1=10.2.1.4
C2=10.2.1.7
UNIVERSE=10.2.2.0/24

start_suite "Ping over cross-host weave network (with and without IPAM)"

weave_on $HOST1 launch -iprange $UNIVERSE
weave_on $HOST2 launch -iprange $UNIVERSE $HOST1

start_container $HOST1 $C1/24 --name=c1
start_container $HOST2 $C2/24 --name=c2
assert_raises "exec_on $HOST1 c1 $PING $C2"

start_container $HOST1 --name=c3
start_container $HOST2 --name=c4
C4=$(container_ip $HOST2 c4)
assert_raises "exec_on $HOST1 c3 $PING $C4"

end_suite
