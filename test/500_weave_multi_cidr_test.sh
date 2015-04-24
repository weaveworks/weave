#! /bin/bash

. ./config.sh

start_suite "Weave run/start/attach/detach with multiple cidr arguments"

weave_on $HOST1 launch
weave_on $HOST1 launch-dns 10.254.254.254/24

# weave_status_on <host>
weave_status_on() {
    HOST=$1; shift

    DOCKER_HOST=tcp://$HOST:2375 $WEAVE status
}

# weave_ps_on <host>
weave_ps_on() {
    HOST=$1; shift

    DOCKER_HOST=tcp://$HOST:2375 $WEAVE ps
}

# assert_container_cidrs <host> <cid> <cidr> [<cidr> ...]
assert_container_cidrs() {
    HOST=$1; shift
    CID=$1; shift
    CIDRS=$@

    # Assert container has attached CIDRs
    assert_raises "weave_ps_on $HOST | grep \"^$CID [^ ]* $CIDRS$\""
}

# assert_zone_records <host> <cid> <fqdn> <ip> [<ip> ...]
assert_zone_records() {
    HOST=$1; shift
    CID=$1; shift
    FQDN=$1; shift

    # Assert correct number of records exist
    assert "weave_status_on $HOST | grep \"^$CID\" | wc -l" $#

    # Assert correct records exist
    for IP in $@; do
        assert_raises "weave_status_on $HOST | grep \"^$CID $IP $FQDN$\""
    done
}

# assert_bridge_cidrs <host> <dev> <cidr> [<cidr> ...]
assert_bridge_cidrs() {
    HOST=$1; shift
    DEV=$1; shift
    CIDRS=$@

    BRIDGE_CIDRS=$($SSH $HOST ip addr show dev $DEV | grep -o 'inet .*' | cut -d ' ' -f 2)

    assert "echo $BRIDGE_CIDRS" "$CIDRS"
}

# Run container with three cidrs
CID=$(DOCKER_HOST=tcp://$HOST1:2375 $WEAVE run 10.2.1.1/24 10.2.2.1/24 10.2.3.1/24 -t --name multicidr -h multicidr.weave.local ubuntu | cut -b 1-12)
assert_container_cidrs $HOST1 $CID 10.2.1.1/24 10.2.2.1/24 10.2.3.1/24
assert_zone_records $HOST1 $CID multicidr.weave.local. 10.2.1.1 10.2.2.1 10.2.3.1

# Remove two of them
weave_on $HOST1 detach 10.2.1.1/24 10.2.3.1/24 $CID
assert_container_cidrs $HOST1 $CID 10.2.2.1/24
assert_zone_records $HOST1 $CID multicidr.weave.local. 10.2.2.1

# Put them both back
weave_on $HOST1 attach 10.2.1.1/24 10.2.3.1/24 $CID
assert_container_cidrs $HOST1 $CID 10.2.2.1/24 10.2.1.1/24 10.2.3.1/24
assert_zone_records $HOST1 $CID multicidr.weave.local. 10.2.2.1 10.2.1.1 10.2.3.1

# Stop the container, restart with three IPs
docker_on $HOST1 stop $CID
weave_on $HOST1 start 10.2.1.1/24 10.2.2.1/24 10.2.3.1/24 $CID
assert_container_cidrs $HOST1 $CID 10.2.1.1/24 10.2.2.1/24 10.2.3.1/24
assert_zone_records $HOST1 $CID multicidr.weave.local. 10.2.1.1 10.2.2.1 10.2.3.1

# Expose some cidrs
weave_on $HOST1 expose 10.2.1.2/24 10.2.2.2/24 10.2.3.2/24
assert_bridge_cidrs $HOST1 weave 10.2.1.2/24 10.2.2.2/24 10.2.3.2/24

# Hide some cidrs
weave_on $HOST1 hide 10.2.1.2/24 10.2.3.2/24
assert_bridge_cidrs $HOST1 weave 10.2.2.2/24

end_suite
