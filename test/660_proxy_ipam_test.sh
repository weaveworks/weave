#! /bin/bash

. ./config.sh

UNIVERSE=10.2.2.0/24

start_suite "Ping proxied containers over cross-host weave network (with IPAM)"

weave_on $HOST1 launch -iprange $UNIVERSE
weave_on $HOST1 launch-proxy
weave_on $HOST2 launch -iprange $UNIVERSE $HOST1
weave_on $HOST2 launch-proxy

proxy start_container $HOST1 -e WEAVE_CIDR= --name=c1
proxy start_container $HOST2 -e WEAVE_CIDR= --name=c2
C2=$(container_ip $HOST2 c2)
assert_raises "exec_on $HOST1 c1 $PING $C2"

end_suite
