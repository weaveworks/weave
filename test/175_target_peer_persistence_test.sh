#! /bin/bash

. ./config.sh


assert_targets() {
    HOST=$1
    shift
    EXPECTED=$(for TARGET in $@; do echo $TARGET; done | sort)
    assert "weave_on $HOST report | jq -r '.Router.Targets[] | tostring' | sort" "$EXPECTED"
}

start_suite "Check Docker restart uses persisted peer list"

# Launch router and modify initial peer list
weave_on $HOST1 launch 192.168.48.11 192.168.48.12
weave_on $HOST1 forget 192.168.48.11
weave_on $HOST1 connect 192.168.48.13

# Ensure modified peer list is still in effect after restart
check_restart $HOST1 weave
assert_targets $HOST1 192.168.48.12 192.168.48.13

# Ensure persisted peer changes are ignored after stop and subsequent restart
weave_on $HOST1 stop
weave_on $HOST1 launch 192.168.48.11 192.168.48.12
assert_targets $HOST1 192.168.48.11 192.168.48.12
check_restart $HOST1 weave
assert_targets $HOST1 192.168.48.11 192.168.48.12

end_suite
