#! /bin/bash

. ./config.sh

C1=10.2.1.1
C2=10.2.1.2
C3=10.2.1.128

assert_migration() {
    start_container $HOST1 $C1/24 --name=c1
    start_container $HOST1 $C2/24 --name=c2
    start_container $HOST2 $C3/24 --name=c3 --privileged

    assert_raises "exec_on $HOST2 c3 $PING $C2"
    MAC=$(exec_on $HOST1 c1 ip link show ethwe | sed -n -e 's|^ *link/ether \([0-9a-f:]*\).*|\1|p')
    docker_on $HOST1 rm -f c1
    exec_on $HOST2 c3 ip link set ethwe address $MAC
    assert_raises "exec_on $HOST2 c3 $PING $C2"
}

start_suite "Container MAC migration"

# Test with fastdp

weave_on $HOST1 launch
weave_on $HOST2 launch $HOST1
assert_migration

# Cleanup

docker_on $HOST1 rm -f c2
docker_on $HOST2 rm -f c3
weave_on $HOST1 reset
weave_on $HOST2 reset

# Test with sleeve

WEAVE_NO_FASTDP=1 weave_on $HOST1 launch
WEAVE_NO_FASTDP=1 weave_on $HOST2 launch $HOST1
assert_migration

end_suite
