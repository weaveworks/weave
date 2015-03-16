#! /bin/bash

. ./config.sh

C1=10.2.3.78
C2=10.2.3.34

start_suite "Resolve names across hosts"

for host in $HOST1 $HOST2; do
    run_on $host sudo $WEAVE stop || true
    run_on $host sudo $WEAVE stop-dns || true
    docker_on $host rm -f c1 c2 || true
done

run_on $HOST1 sudo $WEAVE launch
run_on $HOST2 sudo $WEAVE launch $HOST1

run_on $HOST1 sudo $WEAVE launch-dns 10.2.254.1/24 -debug
run_on $HOST2 sudo $WEAVE launch-dns 10.2.254.2/24 -debug

run_on $HOST2 sudo $WEAVE run $C2/24 -t --name=c2 -h seetwo.weave.local ubuntu
run_on $HOST1 sudo $WEAVE run --with-dns $C1/24 --name=c1 -t aanand/docker-dnsutils /bin/sh

ok=$(docker -H tcp://$HOST1:2375 exec -i c1 dig +short seetwo.weave.local)
assert "echo $ok" "$C2"

ok=$(docker -H tcp://$HOST1:2375 exec -i c1 dig +short -x $C2)
assert "echo $ok" "seetwo.weave.local."

ok=$(docker -H tcp://$HOST1:2375 exec -i c1 dig +short -x 8.8.8.8)
assert "test -n \"$ok\" && echo pass" "pass"

end_suite
