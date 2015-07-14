#! /bin/bash

. ./config.sh

C1=10.2.0.78
C2=10.2.0.34
NAME=seetwo.weave.local
DNS1=10.2.254.1

start_suite "Resolve names on a single host"

launch_dns_on $HOST1 $DNS1/24

start_container          $HOST1 $C2/24 --name=c2 -h $NAME
start_container_with_dns $HOST1 $C1/24 --name=c1

assert_dns_record $HOST1 c1 $NAME $C2
assert_dns_record $HOST1 c1 "dns.weave.local" $DNS1

assert_raises "exec_on $HOST1 c1 dig MX $NAME | grep -q 'status: NXDOMAIN'"

end_suite
