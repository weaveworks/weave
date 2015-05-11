#! /bin/bash

. ./config.sh

C1=10.2.0.78
C2=10.2.0.34
NAME=seetwo.weave.local

start_suite "Resolve names on a single host"

weave_on $HOST1 launch-dns 10.2.254.1/24

weave_on $HOST1 run $C2/24 -d -t --name=c2 -h $NAME gliderlabs/alpine /bin/sh
weave_on $HOST1 run --with-dns $C1/24 -d -t --name=c1 aanand/docker-dnsutils /bin/sh

assert_dns_record $HOST1 c1 $NAME $C2

assert_dns_status $HOST1 c1 "MX   seetwo.weave.local" NXDOMAIN
assert_dns_status $HOST1 c1 "AAAA seetwo.weave.local" NXDOMAIN

end_suite
