#!/bin/bash

. ./config.sh

C1=10.2.1.4
C2=10.2.1.7

start_suite "Ping over encrypted cross-host weave network"

weave_on $HOST1 launch -password notverysecure
weave_on $HOST2 launch -password notverysecure $HOST1

weave_on $HOST2 run $C2/24 -d -t --name=c2 gliderlabs/alpine /bin/sh
weave_on $HOST1 run $C1/24 -d -t --name=c1 gliderlabs/alpine /bin/sh
assert_raises "exec_on $HOST1 c1 ping -q -c 4 $C2"

end_suite
