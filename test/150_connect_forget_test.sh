#! /bin/bash

. ./config.sh

C1=10.2.1.4
C2=10.2.1.7

direct_peers() {
  weave_on $1 status | awk "/^Direct Peers:$/{f=1;next} /Reconnects:/{f=0} f"
}

assert_connected() {
  assert_raises "direct_peers $HOST2 | grep '^$HOST1$'"
  assert_raises "exec_on $HOST1 c1 $PING $C2"
}

assert_disconnected() {
  assert "direct_peers $HOST2" ""
  assert_raises "exec_on $HOST1 c1 sh -c '! $PING $C2'"
}

start_suite "Connecting and forgetting routers after launch"

weave_on $HOST1 launch
weave_on $HOST2 launch

start_container $HOST1 $C1/24 --name=c1
start_container $HOST2 $C2/24 --name=c2

assert_disconnected
assert_raises "weave_on $HOST2 connect $HOST1 $HOST2"
assert_connected

assert_raises "weave_on $HOST2 forget $HOST1 $HOST2"
assert_raises "weave_on $HOST1 stop"
assert_raises "weave_on $HOST1 launch"

assert_disconnected

end_suite
