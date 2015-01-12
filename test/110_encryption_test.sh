#!/bin/bash

. ./config.sh

C1=10.1.1.4
C2=10.1.1.7

start_suite "Ping over cross-host weave network"

for HOST in $HOST1 $HOST2; do
    run_on $HOST sudo $WEAVE stop || true
    run_on $HOST sudo $WEAVE stop-dns || true
    docker_on $HOST rm -f c1 c2 || true
done

run_on $HOST1 sudo $WEAVE launch -password notverysecure
run_on $HOST2 sudo $WEAVE launch -password notverysecure $HOST1

run_on $HOST2 sudo $WEAVE run $C2/24 -t --name=c2 ubuntu
run_on $HOST1 sudo $WEAVE run $C1/24 -t --name=c1 ubuntu
ok=$(docker -H tcp://$HOST1:2375 exec -i c1 sh -c "ping -q -c 4 $C2 >&2 && echo ok")
assert "echo $ok" "ok"

end_suite
