#! /bin/bash

. ./config.sh

NAME=seetwo.weave.local

N=50
start_suite "Add $N dns entries and check we get the right response."

weave_on $HOST1 launch

CID=$(start_container_with_dns $HOST1 10.2.1.0/24 --name=c0)
IPS=""
for i in $(seq $N); do
    IPS="$IPS 10.2.1.$i"
done
weave_on $HOST1 dns-add $IPS $CID -h $NAME

assert_dns_record $HOST1 c0 $NAME $IPS

assert_raises "exec_on $HOST1 c0 dig MX $NAME | grep -q 'status: NXDOMAIN'"

end_suite
