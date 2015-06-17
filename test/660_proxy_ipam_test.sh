#! /bin/bash

. ./config.sh

UNIVERSE=10.2.2.0/24

start_suite "Ping proxied containers over cross-host weave network (with IPAM)"

weave_on $HOST1 launch -iprange $UNIVERSE
weave_on $HOST1 launch-proxy
weave_on $HOST2 launch -iprange $UNIVERSE $HOST1
weave_on $HOST2 launch-proxy --no-default-ipam

proxy docker_on $HOST1 run                --name=auto     -dt $SMALL_IMAGE /bin/sh
proxy docker_on $HOST2 run -e WEAVE_CIDR= --name=explicit -dt $SMALL_IMAGE /bin/sh
proxy docker_on $HOST2 run                --name=none     -dt $SMALL_IMAGE /bin/sh

AUTO=$(container_ip $HOST1 auto)
EXPLICIT=$(container_ip $HOST2 explicit)
assert_raises "proxy exec_on $HOST1 auto     $PING $EXPLICIT"
assert_raises "proxy exec_on $HOST2 explicit $PING $AUTO"

assert_raises "container_ip $HOST2 none" 1
assert_raises "proxy exec_on $HOST2 none ip link show | grep -v ethwe"

end_suite
