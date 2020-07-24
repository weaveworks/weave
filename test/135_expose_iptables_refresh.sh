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
    # Canary is checked every 10s
    sleep 12
}

start_suite "exposing weave network to host"

# Launch
weave_on1 "launch"
IPT_BEFORE=$(get_weave_iptable_rules)
run_on1 "sudo iptables -t nat -X WEAVE-CANARY"
wait_for_iptable_refresh
IPT_AFTER=$(get_weave_iptable_rules)
assert_raises "diff <(echo $IPT_BEFORE) <(echo $IPT_AFTER)" 0
stop_weave_on1

# After exposing
weave_on1 "launch"
weave_on1 "expose"
IPT_BEFORE=$(get_weave_iptable_rules)
run_on1 "sudo iptables -t nat -X WEAVE-CANARY"
wait_for_iptable_refresh
IPT_AFTER=$(get_weave_iptable_rules)
assert_raises "diff <(echo $IPT_BEFORE) <(echo $IPT_AFTER)" 0
stop_weave_on1

end_suite
