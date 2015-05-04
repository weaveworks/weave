#! /bin/bash

. ./config.sh

C1=c1:10.2.1.34
C2=c2:10.2.1.37
C3=c3:10.2.2.34
C4=c4:10.2.2.37
EXP=10.2.1.101

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
    exec_on1 "${C1%:*} $PING ${C2#*:}"
    exec_on1 "${C3%:*} $PING ${C4#*:}"
    # fails due to #620
    # exec_on1 "${C3#:} ! $PING ${C1#*:}"
}

start_suite "exposing weave network to host"

weave_on $HOST1 launch

for c in $C1 $C2 $C3 $C4; do
    weave_on $HOST1 run ${c#*:}/24 -t --name=${c%:*} ubuntu
done

# absence of host connectivity by default
run_on1 "! $PING ${C1#*:}"
check_container_connectivity

# host connectivity after 'expose'
weave_on1 "expose $EXP/24"
run_on1   "  $PING ${C1#*:}"
run_on1   "! $PING ${C3#*:}"
check_container_connectivity

# idempotence of 'expose'
weave_on1 "expose $EXP/24"
run_on1   "$PING ${C1#*:}"

# no host connectivity after 'hide'
weave_on1 "hide $EXP/24"
run_on1   "! $PING ${C1#*:}"

# idempotence of 'hide'
weave_on1 "hide $EXP/24"

end_suite
