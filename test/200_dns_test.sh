#! /bin/bash

. ./config.sh

C1=10.2.0.78
C2=10.2.0.34

start_suite "Resolve names"

run_on $HOST1 sudo $WEAVE stop || true
run_on $HOST1 sudo $WEAVE stop-dns || true
run_on $HOST1 sudo $WEAVE launch-dns 10.0.0.2/8

docker_on $HOST1 rm -f c1 c2 || true

run_on $HOST1 sudo $WEAVE run $C2/24 -t --name=c2 -h seetwo.weave.local ubuntu
run_on $HOST1 sudo $WEAVE run --with-dns $C1/24 -t --name=c1 ubuntu

ok=$(docker -H tcp://$HOST1:2375 exec -i c1 sh -c "ping -q -c4 seetwo.weave.local >&2 && echo ok")
assert "echo $ok" "ok"

end_suite
