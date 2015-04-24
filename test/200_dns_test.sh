#! /bin/bash

. ./config.sh

C1=10.2.0.78
C2=10.2.0.34
NAME=seetwo.weave.local

start_suite "Resolve names on a single host"

weave_on $HOST1 launch-dns 10.2.254.1/24

weave_on $HOST1 run $C2/24 -t --name=c2 -h $NAME ubuntu
weave_on $HOST1 run --with-dns $C1/24 -t --name=c1 aanand/docker-dnsutils /bin/sh

assert_dns_record $HOST1 c1 $NAME $C2

end_suite
