#! /bin/bash

. ./config.sh

start_suite "Test CNI plugin"

coverage_args() {
    # no coverage reporting for now; but when the plugin binary has
    # been built for coverage reporting we need to prevent it from
    # complaining about the non-coverage args.
    [ -n "$COVERAGE" ] && echo "--"
}

cni_connect() {
    pid=$(container_pid $1 $2)
    docker_on $1 run --rm --privileged --net=host --pid=host -i \
        -e CNI_VERSION=1 -e CNI_COMMAND=ADD -e CNI_CONTAINERID=c1 \
        -e CNI_NETNS=/proc/$pid/ns/net -e CNI_IFNAME=eth0 -e CNI_PATH=/opt/cni/bin \
        weaveworks/plugin:latest $(coverage_args) --cni-net <<EOF
{
    "name": "weave",
    "type": "weave-net"
}
EOF
}

weave_on $HOST1 launch
weave_on $HOST1 expose

C1=$(docker_on $HOST1 run --net=none --name=c1 -dt $SMALL_IMAGE /bin/sh)
C2=$(docker_on $HOST1 run --net=none --name=c2 -dt $SMALL_IMAGE /bin/sh)

cni_connect $HOST1 c1 >/dev/null
cni_connect $HOST1 c2 >/dev/null

C1IP=$(container_ip $HOST1 c1)
C2IP=$(container_ip $HOST1 c2)

assert_raises "exec_on $HOST1 c1 $PING $C2IP"
assert_raises "exec_on $HOST1 c2 $PING $C1IP"

end_suite
