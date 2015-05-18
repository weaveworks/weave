#! /bin/bash

. ./config.sh

UNIVERSE=10.2.1.0/24

start_suite "Ping over cross-host weave network with IPAM"

weave_on $HOST1 launch -iprange $UNIVERSE
weave_on $HOST2 launch -iprange $UNIVERSE $HOST1

start_container $HOST1 --name=c1
start_container $HOST2 --name=c2
C2=$(container_ip $HOST2 c2)
assert_raises "exec_on $HOST1 c1 $PING $C2"

end_suite
