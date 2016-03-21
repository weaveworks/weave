#! /bin/bash

. ./config.sh

start_suite "Test docker restart policy"

check_restart() {
    OLD_PID=$(container_pid $1 $2)

    run_on $1 sudo kill $OLD_PID

    for i in $(seq 1 30); do
        NEW_PID=$(container_pid $1 $2)

        if [ $NEW_PID != 0 -a $NEW_PID != $OLD_PID ] ; then
            return 0
        fi

        sleep 1
    done

    return 1
}

weave_on $HOST1 launch

assert_raises "check_restart $HOST1 weave"
assert_raises "check_restart $HOST1 weaveproxy"

end_suite
