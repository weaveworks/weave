#! /bin/bash

. ./config.sh

C1=10.2.3.78
C2=10.2.3.34
C2a=10.2.3.35
C2b=10.2.3.36
UNIVERSE=10.2.4.0/24
NAME2=seetwo.weave.local
NAME4=seefour.weave.local

start_suite "Resolve names across hosts"

weave_on $HOST1 launch --ipalloc-range $UNIVERSE
weave_on $HOST2 launch --ipalloc-range $UNIVERSE $HOST1

start_container          $HOST2 $C2/24 --name=c2 -h $NAME2
start_container_with_dns $HOST1 $C1/24 --name=c1

assert_dns_record $HOST1 c1 $NAME2 $C2

# resolution for names mapped to multiple addresses
weave_on $HOST2 dns-add $C2a c2 -h $NAME2
weave_on $HOST2 dns-add $C2b c2 -h $NAME2
assert_dns_record $HOST1 c1 $NAME2 $C2 $C2a $C2b

# resolution when containers addresses come from IPAM
start_container          $HOST2 --name=c4 -h $NAME4
start_container_with_dns $HOST1 --name=c3
C4=$(container_ip $HOST2 c4)
assert_dns_record $HOST1 c3 $NAME4 $C4
assert_raises "exec_on $HOST1 c3 getent hosts 8.8.8.8 | grep google"

end_suite
