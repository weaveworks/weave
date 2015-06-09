#! /bin/bash

. ./config.sh

C1=10.2.0.78
C2=10.2.0.34
C1_NAME=c1.weave.local
C2_NAME=seetwo.weave.local

start_suite "Proxy registers containers with dns"

weave_on $HOST1 launch-dns 10.2.254.1/24
weave_on $HOST1 launch-proxy --with-dns
proxy docker_on $HOST1 run -e WEAVE_CIDR=$C2/24 -dt --name=c2 -h $C2_NAME $SMALL_IMAGE /bin/sh
proxy docker_on $HOST1 run -e WEAVE_CIDR=$C1/24 -dt --name=c1             $DNS_IMAGE   /bin/sh

assert_dns_record $HOST1 c1 $C2_NAME $C2
assert_dns_record $HOST1 c2 $C1_NAME $C1

end_suite
