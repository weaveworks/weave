#! /bin/bash

. ./config.sh

IP=10.2.0.34
TARGET=seetwo.weave.local
TARGET_IP=10.2.0.78

assert_no_resolution() {
    container=$(weave_on $HOST1 run "$@" $IP/24 -t $DNS_IMAGE /bin/sh)
    assert_no_dns_record $HOST1 $container $TARGET
    rm_containers $HOST1 $container
}

assert_resolution() {
    container=$(weave_on $HOST1 run "$@" $IP/24 -t $DNS_IMAGE /bin/sh)
    assert_dns_record $HOST1 $container $TARGET $TARGET_IP
    rm_containers $HOST1 $container
}

start_suite "With or without DNS test"

weave_on $HOST1 launch-router

DNS_IP=$(weave_on $HOST1 docker-bridge-ip)

# Assert behaviour without weaveDNS running
assert_no_resolution --without-dns
assert_no_resolution
assert_no_resolution --with-dns

weave_on $HOST1 launch-dns 10.2.254.1/24 $WEAVEDNS_ARGS
start_container $HOST1 $TARGET_IP/24 --name c2 -h $TARGET

# Assert behaviour with weaveDNS running
assert_no_resolution --without-dns
assert_resolution
assert_resolution --with-dns

end_suite
