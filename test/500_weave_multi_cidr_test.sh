#! /bin/bash

. ./config.sh

start_suite "Weave run/start/attach/detach with multiple cidr arguments"

# Cleanup from previous run
weave_on $HOST1 stop || true
weave_on $HOST1 stop-dns || true
docker_on $HOST1 rm -f multicidr || true

weave_on $HOST1 launch
weave_on $HOST1 launch-dns 10.254.254.254/24

# status <host>
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

# Run container with three cidrs
CID=$(DOCKER_HOST=tcp://$HOST1:2375 $WEAVE run 10.0.0.1/24 10.0.0.2/24 10.0.0.3/24 -t --name multicidr -h multicidr.weave.local ubuntu | cut -b 1-12)
assert_container_cidrs $HOST1 $CID 10.0.0.1/24 10.0.0.2/24 10.0.0.3/24
assert_zone_records $HOST1 $CID multicidr.weave.local. 10.0.0.1 10.0.0.2 10.0.0.3

# Remove two of them
weave_on $HOST1 detach 10.0.0.2/24 10.0.0.3/24 $CID
assert_container_cidrs $HOST1 $CID 10.0.0.1/24
assert_zone_records $HOST1 $CID multicidr.weave.local. 10.0.0.1

# Put them both back
weave_on $HOST1 attach 10.0.0.2/24 10.0.0.3/24 $CID
assert_container_cidrs $HOST1 $CID 10.0.0.1/24 10.0.0.2/24 10.0.0.3/24
assert_zone_records $HOST1 $CID multicidr.weave.local. 10.0.0.1 10.0.0.2 10.0.0.3

# Stop the container, restart with three IPs
docker_on $HOST1 stop $CID
weave_on $HOST1 start 10.0.0.1/24 10.0.0.2/24 10.0.0.3/24 $CID
assert_container_cidrs $HOST1 $CID 10.0.0.1/24 10.0.0.2/24 10.0.0.3/24
assert_zone_records $HOST1 $CID multicidr.weave.local. 10.0.0.1 10.0.0.2 10.0.0.3

end_suite