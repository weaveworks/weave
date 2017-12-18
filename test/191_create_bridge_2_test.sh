#! /bin/bash

. "$(dirname "$0")/config.sh"

C1=10.32.0.1
C2=10.32.0.2

kill_weaver() {
    run_on $HOST1 sudo ip link set weave down
    WEAVER_PID=$(container_pid $HOST1 weave)
    run_on $HOST1 sudo kill -9 $WEAVER_PID
}

start_suite "Re-create bridge after restart"

# Should create a bridge of the "bridge" type
WEAVE_NO_FASTDP=1 weave_on $HOST1 launch
WEAVE_NO_FASTDP=1 weave_on $HOST2 launch $HOST1

start_container $HOST1 $C1/24 --name=c1
start_container $HOST2 $C2/24 --name=c2
assert_raises "exec_on $HOST1 c1 $PING $C2"

kill_weaver # should re-create the bridge

sleep 3

assert_raises "exec_on $HOST1 c1 $PING $C2"

# Should create a bridge of the "bridged_fastdp" type
weave_on $HOST1 reset
weave_on $HOST1 launch $HOST2
weave_on $HOST1 attach $C1/24 c1

assert_raises "exec_on $HOST1 c1 $PING $C2"

kill_weaver # should re-create the bridge

sleep 3

assert_raises "exec_on $HOST1 c1 $PING $C2"

end_suite
