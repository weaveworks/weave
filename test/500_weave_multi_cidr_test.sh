#! /bin/bash

. ./config.sh

NAME=multicidr.weave.local

# assert_container_cidrs <host> <cid> [<cidr> ...]
assert_container_cidrs() {
    HOST=$1
    CID=$2
    shift 2
    CIDRS="$@"

    # Assert container has attached CIDRs
    if [ -z "$CIDRS" ] ; then
        assert        "weave_on $HOST ps $CID" ""
    else
        assert_raises "weave_on $HOST ps $CID | grep -E '^$CID [0-9a-f:]{17} $CIDRS$'"
    fi
}

# assert_zone_records <host> <cid> <fqdn> [<ip>|<cidr> ...]
assert_zone_records() {
    HOST=$1
    CID=$2
    FQDN=$3
    shift 3

    records=$(weave_on $HOST status | grep "^$CID") || true
    # Assert correct number of records exist
    assert "echo $records | grep -oE '\b([0-9]{1,3}\.){3}[0-9]{1,3}\b' | wc -l" $#

    # Assert correct records exist
    for ADDR; do
        assert_raises "echo $records | grep '${ADDR%/*}' | grep '$FQDN'"
    done
}

# assert_bridge_cidrs <host> <dev> <cidr> [<cidr> ...]
assert_bridge_cidrs() {
    HOST=$1
    DEV=$2
    shift 2
    CIDRS="$@"

    BRIDGE_CIDRS=$($SSH $HOST ip addr show dev $DEV | grep -o 'inet .*' | cut -d ' ' -f 2)
    assert "echo $BRIDGE_CIDRS" "$CIDRS"
}

start_suite "Weave run/start/attach/detach/expose/hide with multiple cidr arguments"

# also check that these commands understand all address flavours

# NOTE: in these tests, net: arguments are checked against a
# specific address, i.e. we are assuming that IPAM always returns the
# lowest available address in the subnet

weave_on $HOST1 launch -debug -iprange 10.2.3.0/24
launch_dns_on $HOST1 10.254.254.254/24

# Run container with three cidrs
CID=$(start_container  $HOST1             10.2.1.1/24 ip:10.2.2.1/24 net:10.2.3.0/24 --name=multicidr -h $NAME | cut -b 1-12)
assert_container_cidrs $HOST1 $CID        10.2.1.1/24    10.2.2.1/24     10.2.3.1/24
assert_zone_records    $HOST1 $CID $NAME. 10.2.1.1       10.2.2.1        10.2.3.1

# Stop the container
docker_on              $HOST1 stop $CID
assert_container_cidrs $HOST1 $CID
assert_zone_records    $HOST1 $CID $NAME.

# Restart with three IPs
weave_on               $HOST1 start       10.2.1.1/24 ip:10.2.2.1/24 net:10.2.3.0/24 $CID
assert_container_cidrs $HOST1 $CID        10.2.1.1/24    10.2.2.1/24     10.2.3.1/24
assert_zone_records    $HOST1 $CID $NAME. 10.2.1.1       10.2.2.1        10.2.3.1

# Remove two of them
weave_on               $HOST1 detach                  ip:10.2.2.1/24 net:10.2.3.0/24 $CID
assert_container_cidrs $HOST1 $CID        10.2.1.1/24
assert_zone_records    $HOST1 $CID $NAME. 10.2.1.1
# ...and the remaining one
weave_on               $HOST1 detach      10.2.1.1/24                                $CID
assert_container_cidrs $HOST1 $CID
assert_zone_records    $HOST1 $CID $NAME.

# Put one back
weave_on               $HOST1 attach      10.2.1.1/24                                $CID
assert_container_cidrs $HOST1 $CID        10.2.1.1/24
assert_zone_records    $HOST1 $CID $NAME. 10.2.1.1
# ...and the remaining two
weave_on               $HOST1 attach                  ip:10.2.2.1/24 net:10.2.3.0/24 $CID
assert_container_cidrs $HOST1 $CID        10.2.1.1/24    10.2.2.1/24     10.2.3.1/24
assert_zone_records    $HOST1 $CID $NAME. 10.2.1.1       10.2.2.1        10.2.3.1

# Expose three cidrs
weave_on               $HOST1 expose      10.2.1.2/24 ip:10.2.2.2/24 net:10.2.3.0/24
assert_bridge_cidrs    $HOST1 weave       10.2.1.2/24    10.2.2.2/24     10.2.3.2/24

# Hide two of them
weave_on               $HOST1 hide                    ip:10.2.2.2/24 net:10.2.3.0/24
assert_bridge_cidrs    $HOST1 weave       10.2.1.2/24
# ...and the remaining one
weave_on               $HOST1 hide        10.2.1.2/24
assert_bridge_cidrs    $HOST1 weave

# Now detach and run another container to check we have released IPs in IPAM
weave_on               $HOST1 detach $CID
CID2=$(start_container $HOST1         net:10.2.3.0/24)
assert_container_cidrs $HOST1 $CID2       10.2.3.1/24

end_suite
