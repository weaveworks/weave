#! /bin/bash

. ./config.sh

UNIVERSE=10.2.1.0/24

start_suite "Ping over cross-host weave network with IPAM"

for HOST in $HOST1 $HOST2; do
    weave_on $HOST stop || true
    weave_on $HOST stop-dns || true
    docker_on $HOST rm -f c1 c2 || true
done

weave_on $HOST1 launch -alloc $UNIVERSE -debug
weave_on $HOST2 launch -alloc $UNIVERSE -debug $HOST1

weave_on $HOST1 run -t --name=c1 ubuntu
weave_on $HOST2 run -t --name=c2 ubuntu
# Note can't use weave_on here because it echoes the command
C2IP=$(DOCKER_HOST=tcp://$HOST2:2375 $WEAVE ps | grep -o -E '[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}')
ok=$(docker -H tcp://$HOST1:2375 exec -i c1 sh -c "ping -q -c 4 $C2IP >&2 && echo ok")
assert "echo $ok" "ok"

end_suite
