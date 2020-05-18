#!/bin/bash

. "$(dirname "$0")/config.sh"

run_on1() {
    assert_raises "run_on $HOST1 $@"
}

weave_on1() {
    assert_raises "weave_on $HOST1 $@"
}

stop_weave_on1() {
    assert_raises "stop_weave_on $HOST1 $@"
}

get_weave_iptable_rules() {
    get_command_output_on $HOST1 "sudo iptables-save | grep -i weave"
}

wait_for_iptable_refresh() {
    sleep 2
}

start_suite "exposing weave network to host"

# Launch

## Check no refreshing
weave_on1 "launch --iptables-refresh-interval=0s"
IPT_BEFORE=$(get_weave_iptable_rules)
run_on1 "sudo iptables -t nat -X WEAVE-CANARY"
wait_for_iptable_refresh
IPT_AFTER=$(get_weave_iptable_rules)
assert_raises "diff <(echo $IPT_BEFORE) <(echo $IPT_AFTER)" 1
stop_weave_on1

## Check refreshing
weave_on1 "launch --iptables-refresh-interval=1s"
IPT_BEFORE=$(get_weave_iptable_rules)
run_on1 "sudo iptables -t nat -X WEAVE-CANARY"
wait_for_iptable_refresh
IPT_AFTER=$(get_weave_iptable_rules)
assert_raises "diff <(echo $IPT_BEFORE) <(echo $IPT_AFTER)" 0
stop_weave_on1

# Expose

## Check no refreshing
weave_on1 "launch --iptables-refresh-interval=0s"
weave_on1 "expose"
IPT_BEFORE=$(get_weave_iptable_rules)
run_on1 "sudo iptables -t nat -X WEAVE-CANARY"
wait_for_iptable_refresh
IPT_AFTER=$(get_weave_iptable_rules)
assert_raises "diff <(echo $IPT_BEFORE) <(echo $IPT_AFTER)" 1
stop_weave_on1

## Check refreshing
weave_on1 "launch --iptables-refresh-interval=1s"
weave_on1 "expose"
IPT_BEFORE=$(get_weave_iptable_rules)
run_on1 "sudo iptables -t nat -X WEAVE-CANARY"
wait_for_iptable_refresh
IPT_AFTER=$(get_weave_iptable_rules)
assert_raises "diff <(echo $IPT_BEFORE) <(echo $IPT_AFTER)" 0
stop_weave_on1

end_suite
