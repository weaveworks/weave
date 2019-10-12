#! /bin/bash

. "$(dirname "$0")/config.sh"

C1=10.32.0.2
C2=10.32.0.3

kill_weaver() {
    run_on $HOST1 sudo ip link set weave down
    WEAVER_PID=$(container_pid $HOST1 weave)
    run_on $HOST1 sudo kill -9 $WEAVER_PID
    sleep 5
}

start_suite "Re-create bridge after restart"

# Should create a bridge of the "bridge" type.
WEAVE_NO_FASTDP=1 weave_on $HOST1 launch
WEAVE_NO_FASTDP=1 weave_on $HOST2 launch $HOST1

start_container $HOST1 $C1/24 --name=c1
start_container $HOST2 $C2/24 --name=c2
sleep 2 # give topology gossip some time to propagate
assert_raises "exec_on $HOST1 c1 $PING $C2"

kill_weaver # should re-create the bridge

assert_raises "exec_on $HOST1 c1 $PING $C2"

# Should create a bridge of the "bridged_fastdp" type
weave_on $HOST1 reset
weave_on $HOST1 launch $HOST2
weave_on $HOST1 attach $C1/24 c1
sleep 2 # give topology gossip some time to propagate
assert_raises "exec_on $HOST1 c1 $PING $C2"

kill_weaver # should re-create the bridge

assert_raises "exec_on $HOST1 c1 $PING $C2"

# test restore of iptables

weave_on $HOST1 reset
# `--expect-npc` to trigger creation of WEAVE-NPC iptables chain.
weave_on $HOST1 launch --expect-npc
# To create POSTROUTING rules.
weave_on $HOST1 expose

IPT_BEFORE=$(mktemp)
IPT_AFTER=$(mktemp)
run_on $HOST1 "sudo iptables-save | grep -i weave | grep -v '\[.*:.*\]' > $IPT_BEFORE"

run_on $HOST1 "sudo iptables -t filter -D FORWARD -o weave -m comment --comment \"NOTE: this must go before '-j KUBE-FORWARD'\" -j WEAVE-NPC"
run_on $HOST1 sudo iptables -t nat -D POSTROUTING -j WEAVE
kill_weaver # should re-create the bridge and iptables friends

# Rudimentary check that weave related iptables rules has been restored
run_on $HOST1 "sudo iptables-save | grep -i weave | grep -v '\[.*:.*\]' > $IPT_AFTER"
assert_raises "run_on $HOST1 diff $IPT_BEFORE $IPT_AFTER"

end_suite
