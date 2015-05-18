#! /bin/bash

. ./config.sh

C1=10.2.3.78
C2_1=10.2.3.34
C2_2=10.2.3.35
C2_3=10.2.3.36
UNIVERSE=10.2.4.0/24
NAME2=seetwo.weave.local
NAME4=seefour.weave.local

start_suite "Resolve names across hosts (with and without IPAM)"

weave_on $HOST1 launch -iprange $UNIVERSE
weave_on $HOST2 launch -iprange $UNIVERSE $HOST1

weave_on $HOST1 launch-dns 10.2.254.1/24 --no-cache
weave_on $HOST2 launch-dns 10.2.254.2/24 --no-cache

start_container          $HOST2 $C2_1/24 --name=c2 -h $NAME2
start_container          $HOST2          --name=c4 -h $NAME4
start_container_with_dns $HOST1 $C1/24   --name=c1
start_container_with_dns $HOST1          --name=c3
C4=$(container_ip $HOST2 c4)

tell_dns PUT $HOST2 c2 $NAME $C2_2 $NAME2
tell_dns PUT $HOST2 c2 $NAME $C2_3 $NAME2

assert_dns_record $HOST1 c1 $NAME2 $C2_1 $C2_2 $C2_3
assert_dns_record $HOST1 c3 $NAME4 $C4

assert_raises "exec_on $HOST1 c1 getent hosts 8.8.8.8 | grep google"
assert_raises "exec_on $HOST1 c3 getent hosts 8.8.8.8 | grep google"

end_suite
