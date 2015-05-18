#! /bin/bash

. ./config.sh

C1=10.2.3.78
C2=10.2.3.34
NAME=seetwo.weave.local

start_suite "Resolve names across hosts"

weave_on $HOST1 launch
weave_on $HOST2 launch $HOST1

weave_on $HOST1 launch-dns 10.2.254.1/24
weave_on $HOST2 launch-dns 10.2.254.2/24

start_container          $HOST2 $C2/24 --name=c2 -h $NAME
start_container_with_dns $HOST1 $C1/24 --name=c1

assert_dns_record $HOST1 c1 $NAME $C2

assert_raises "exec_on $HOST1 c1 getent hosts 8.8.8.8 | grep google"

end_suite
