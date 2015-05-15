#! /bin/bash

. ./config.sh

UNIVERSE=10.2.1.0/24

start_suite "Ping over cross-host weave network with IPAM"

weave_on $HOST1 launch -iprange $UNIVERSE
weave_on $HOST2 launch -iprange $UNIVERSE $HOST1

weave_on $HOST1 run -t --name=c1 gliderlabs/alpine /bin/sh
weave_on $HOST2 run -t --name=c2 gliderlabs/alpine /bin/sh
# Note can't use weave_on here because it echoes the command
C2IP=$(DOCKER_HOST=tcp://$HOST2:2375 $WEAVE ps | grep -o -E '[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}')
assert_raises "exec_on $HOST1 c1 ping -q -c 4 $C2IP"

end_suite
