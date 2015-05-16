#! /bin/bash

. ./config.sh

UNIVERSE=10.2.3.0/24
NAME=seetwo.weave.local

start_suite "Resolve names over cross-host weave network with IPAM"

weave_on $HOST1 launch -iprange $UNIVERSE
weave_on $HOST2 launch -iprange $UNIVERSE $HOST1

weave_on $HOST1 launch-dns 10.2.254.1/24
weave_on $HOST2 launch-dns 10.2.254.2/24

weave_on $HOST2 run -t --name=c2 -h $NAME gliderlabs/alpine /bin/sh
weave_on $HOST1 run --with-dns --name=c1 -t aanand/docker-dnsutils /bin/sh
C2=$(container_ip $HOST2 c2)

assert_dns_record $HOST1 c1 $NAME $C2

end_suite
