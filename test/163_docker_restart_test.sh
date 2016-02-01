#! /bin/bash

. ./config.sh

check_attached() {
    for c in $@; do
        assert_raises "exec_on $HOST1 $c $CHECK_ETHWE_UP"
    done
}

wait_for_proxy() {
    for i in $(seq 1 120); do
        echo "Waiting for proxy to start"
        if proxy docker_on $1 info > /dev/null 2>&1 ; then
            return
        fi
        sleep 1
    done
    echo "Timed out waiting for proxy to start" >&2
    exit 1
}

start_suite "Containers get same IP address on restart"

# Remove any persisted data from previous runs
run_on $HOST1 "sudo rm -f /tmp/test645-*"

WEAVE_DOCKER_ARGS="-v /tmp:/db --restart=always" weave_on $HOST1 launch-router --db-prefix=/db/test645-
WEAVEPROXY_DOCKER_ARGS=--restart=always weave_on $HOST1 launch-proxy

# Use up first address with throwaway container
start_container $HOST1 --name=c1
# Use sigproxy+sleep to create a container that will die when Docker asks it to.
proxy docker_on $HOST1 run -di --name=c2 --restart=always -dt --entrypoint="/home/weave/sigproxy" weaveworks/weaveexec sleep 600
C2=$(container_ip $HOST1 c2)
assert_raises "[ -n $C2 ]"
check_attached c2

# Another container, that we attach after creation
docker_on $HOST1 run -di --name=c3 --restart=always -dt --entrypoint="/home/weave/sigproxy" weaveworks/weaveexec sleep 600
weave_on $HOST1 attach c3
C3=$(container_ip $HOST1 c3)
assert_raises "[ -n $C3 ]"
check_attached c3

docker_on $HOST1 rm -f c1

# Restart docker daemon, using different commands for systemd- and upstart-managed.
run_on $HOST1 sh -c "command -v systemctl >/dev/null && sudo systemctl restart docker || sudo service docker restart"
wait_for_proxy $HOST1
sleep 3 # allow for re-tries of attach
check_attached c2 c3
# Check same IP address was retained
assert "container_ip $HOST1 c2" "$C2"
assert "container_ip $HOST1 c3" "$C3"

end_suite
