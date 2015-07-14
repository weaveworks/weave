#!/bin/bash

. ./config.sh

C1=10.2.1.4
C2=10.2.1.7

start_suite "Ping over encrypted cross-host weave network"

weave_on $HOST1 launch --password wfvAwt7sj
weave_on $HOST2 launch --password wfvAwt7sj $HOST1

start_container $HOST1 $C1/24 --name=c1
start_container $HOST2 $C2/24 --name=c2
assert_raises "exec_on $HOST1 c1 $PING $C2"

end_suite
