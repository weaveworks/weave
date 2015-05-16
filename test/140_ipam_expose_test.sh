#! /bin/bash

. ./config.sh

R1=10.2.1.0/24
C3=10.2.2.34
C4=10.2.2.37

PING="ping -nq -W 1 -c 1"

weave_on1() {
    assert_raises "weave_on $HOST1 $@"
}

run_on1() {
    assert_raises "run_on   $HOST1 $@"
}

exec_on1() {
    assert_raises "exec_on  $HOST1 $@"
}

check_container_connectivity() {
    exec_on1 "c2 $PING $C1"
    exec_on1 "c3 $PING $C4"
    # fails due to #620
    # exec_on1 "c3 ! $PING $C1"
}

start_suite "exposing weave network to host with IPAM"

weave_on $HOST1 launch -iprange $R1

weave_on $HOST1 run -t --name=c1 gliderlabs/alpine /bin/sh
weave_on $HOST1 run -t --name=c2 gliderlabs/alpine /bin/sh
weave_on $HOST1 run $C3/24 -t --name=c3 gliderlabs/alpine /bin/sh
weave_on $HOST1 run $C4/24 -t --name=c4 gliderlabs/alpine /bin/sh
C1=$(weave_on $HOST1 ps c1 | grep -o -E '[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}')

# absence of host connectivity by default
run_on1 "! $PING $C1"
check_container_connectivity

# host connectivity after 'expose'
weave_on1 "expose"
run_on1   "  $PING $C1"
run_on1   "! $PING $C3"
check_container_connectivity

# idempotence of 'expose'
weave_on1 "expose"
run_on1   "  $PING $C1"

# no host connectivity after 'hide'
weave_on1 "hide"
run_on1   "! $PING $C1"

# idempotence of 'hide'
weave_on1 "hide"

end_suite
