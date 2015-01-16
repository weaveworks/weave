#! /bin/bash

. ./config.sh

start_suite "Resolve a non-weave address"

run_on $HOST1 sudo $WEAVE stop-dns || true
run_on $HOST1 sudo $WEAVE launch-dns 10.0.0.2
docker_on $HOST1 rm -f c1 || true

run_on $HOST1 sudo $WEAVE run --with-dns 10.1.1.5/24 --name=c1 -t aanand/docker-dnsutils /bin/sh

ok=$(docker -H tcp://$HOST1:2375 exec -i c1 dig +short -t MX weave.works)
assert "test -n \"$ok\" && echo pass" "pass"

end_suite
