#! /bin/bash

. ./config.sh

UNIVERSE=10.2.2.0/24

start_suite "Ping proxied containers over cross-host weave network (with IPAM)"

weave_on $HOST1 launch -iprange $UNIVERSE
weave_on $HOST1 launch-proxy
weave_on $HOST2 launch -iprange $UNIVERSE $HOST1
weave_on $HOST2 launch-proxy --with-ipam

proxy docker_on $HOST1 run -e WEAVE_CIDR= --name=c1 -dt $SMALL_IMAGE /bin/sh
proxy docker_on $HOST2 run                --name=c2 -dt $SMALL_IMAGE /bin/sh

C1=$(container_ip $HOST1 c1)
C2=$(container_ip $HOST2 c2)
assert_raises "proxy exec_on $HOST1 c1 $PING $C2"
assert_raises "proxy exec_on $HOST2 c2 $PING $C1"

end_suite
