#! /bin/bash

. ./config.sh

R1=10.2.1.0/24
C3=10.2.2.34
C4=10.2.2.37

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

start_container $HOST1        --name=c1
start_container $HOST1        --name=c2
start_container $HOST1 $C3/24 --name=c3
start_container $HOST1 $C4/24 --name=c4
C1=$(container_ip $HOST1 c1)

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
