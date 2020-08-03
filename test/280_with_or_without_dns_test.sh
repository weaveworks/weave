#! /bin/bash

. "$(dirname "$0")/config.sh"

IP=10.2.0.34
TARGET=seetwo.weave.local
TARGET_IP=10.2.0.78
STATIC=static.name
STATIC_IP=10.9.9.9
ATTACH_ARGS="--rewrite-hosts --add-host=$STATIC:$STATIC_IP"

check_dns() {
    chk=$1
    shift

    container=$(start_container_with_dns $HOST1 "$@" $IP/24)
    $chk $HOST1 $container $TARGET
    # --add-host adds to /etc/hosts so should work even when DNS is off.
    assert_dns_record $HOST1 $container $STATIC $STATIC_IP
    rm_containers $HOST1 $container
}

start_suite "With or without DNS test"

# Assert behaviour without weaveDNS running
weave_on $HOST1 launch --no-dns

start_container $HOST1 $TARGET_IP/24 --name c2 -h $TARGET

check_dns assert_no_dns_record --without-dns
check_dns assert_no_dns_record

weave_on $HOST1 stop

# Assert behaviour with weaveDNS
weave_on $HOST1 launch

check_dns assert_no_dns_record --without-dns
check_dns assert_dns_record

end_suite
