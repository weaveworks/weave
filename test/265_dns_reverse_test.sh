#! /bin/bash

. ./config.sh

IP=10.2.0.67

start_suite "Reverse DNS test"

weave_on $HOST1 launch
start_container_with_dns $HOST1 --name=c1

for name in bar bar black sheep have you any wool; do
    start_container $HOST1 $IP/24 --name=$name
    assert "exec_on $HOST1 c1 dig +short -x $IP" $name.weave.local.
    rm_containers $HOST1 $name
done

end_suite
