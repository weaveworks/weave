#! /bin/bash

. "$(dirname "$0")/config.sh"

start_suite "Test CNI plugin with static IP allocation"

cni_connect() {
    pid=$(container_pid $1 $2)
    id=$(docker_on $1 inspect -f '{{.Id}}' $2)
    run_on $1 sudo CNI_COMMAND=ADD CNI_CONTAINERID=$id CNI_IFNAME=eth0 \
    CNI_NETNS=/proc/$pid/ns/net CNI_PATH=/opt/cni/bin /opt/cni/bin/weave-net
}

run_on $HOST1 sudo mkdir -p /opt/cni/bin
# setup-cni is a subset of 'weave setup', without doing any 'docker pull's
weave_on $HOST1 setup-cni --log-level=debug
weave_on $HOST1 launch --log-level=debug

C0=$(docker_on $HOST1 run --net=none --name=c0 --privileged -dt $SMALL_IMAGE /bin/sh)
C1=$(docker_on $HOST1 run --net=none --name=c1 --privileged -dt $SMALL_IMAGE /bin/sh)
C2=$(docker_on $HOST1 run --net=none --name=c2 --privileged -dt $SMALL_IMAGE /bin/sh)

# Enable unsolicited ARPs so that ping after the address reuse does not time out
exec_on $HOST1 c1 sysctl -w net.ipv4.conf.all.arp_accept=1

cni_connect $HOST1 c0 <<EOF
{
    "name": "weave",
    "type": "weave-net",
    "ipam": {
        "type": "weave-ipam",
        "ips": [
          "10.32.1.30"
        ]
    },
    "hairpinMode": true
}
EOF

cni_connect $HOST1 c1 <<EOF
{
    "name": "weave",
    "type": "weave-net",
    "ipam": {
        "type": "weave-ipam",
        "ips": [
          "10.32.1.40"
        ]
    },
    "hairpinMode": true
}
EOF

cni_connect $HOST1 c2 <<EOF
{
    "name": "weave",
    "type": "weave-net",
    "ipam": {
        "type": "weave-ipam",
        "ips": [
          "10.32.1.42"
        ]
    },
    "hairpinMode": true
}
EOF


C0IP=$(container_ip $HOST1 c0)
C1IP=$(container_ip $HOST1 c1)
C2IP=$(container_ip $HOST1 c2)

assert_raises "[ "10.32.1.30" == $C0IP ]"
assert_raises "[ "10.32.1.40" == $C1IP ]"
assert_raises "[ "10.32.1.42" == $C2IP ]"

BRIP=$(container_ip $HOST1 weave:expose)
# Check the bridge IP is different from the container IPs
assert_raises "[ $BRIP != $C0IP ]"
assert_raises "[ $BRIP != $C1IP ]"
assert_raises "[ $BRIP != $C2IP ]"

# Containers should be able to reach one another
assert_raises "exec_on $HOST1 c0 $PING $C1IP"
assert_raises "exec_on $HOST1 c1 $PING $C2IP"
assert_raises "exec_on $HOST1 c2 $PING $C1IP"

# Containers should not have a default route to the world
assert_raises "exec_on $HOST1 c0 sh -c '! $PING 8.8.8.8'"
assert_raises "exec_on $HOST1 c1 sh -c '! $PING 8.8.8.8'"
assert_raises "exec_on $HOST1 c2 sh -c '! $PING 8.8.8.8'"

# Ensure existing containers can reclaim their IP addresses after CNI has been used -- see #2548
stop_weave_on $HOST1

# Ensure no warning is printed to the standard error:
ACTUAL_OUTPUT=$(CHECKPOINT_DISABLE="$CHECKPOINT_DISABLE" DOCKER_HOST=tcp://$HOST1:$DOCKER_PORT $WEAVE launch 2>&1)
EXPECTED_OUTPUT=$($SSH $HOST1 docker inspect --format="{{.Id}}" weave)

assert_raises "[ $EXPECTED_OUTPUT == $ACTUAL_OUTPUT ]"

assert "$SSH $HOST1 \"curl -s -X GET 127.0.0.1:6784/ip/$C1 | grep -o -E '[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}'\"" "$C1IP"
assert "$SSH $HOST1 \"curl -s -X GET 127.0.0.1:6784/ip/$C2 | grep -o -E '[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}'\"" "$C2IP"
assert "$SSH $HOST1 \"curl -s -X GET 127.0.0.1:6784/ip/$C3 | grep -o -E '[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}'\"" "$C3IP"


end_suite
