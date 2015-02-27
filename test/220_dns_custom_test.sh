#! /bin/bash

. ./config.sh

C1=10.2.56.34
C2=10.2.54.91

start_suite "Resolve names in custom domain"

weave_on $HOST1 stop || true
weave_on $HOST1 stop-dns || true
weave_on $HOST1 launch-dns 10.2.254.1/24 --localDomain foo.bar.

docker_on $HOST1 rm -f c1 c2 || true

weave_on $HOST1 run $C2/24 -t --name=c2 -h seetwo.foo.bar ubuntu
weave_on $HOST1 run --with-dns $C1/24 -t --name=c1 aanand/docker-dnsutils /bin/sh

ok=$(docker -H tcp://$HOST1:2375 exec -i c1 sh -c "dig +short seetwo.foo.bar.")
assert "echo $ok" "$C2"

ok=$(docker -H tcp://$HOST1:2375 exec -i c1 sh -c "dig +short -x $C2")
assert "echo $ok" "seetwo.foo.bar."

end_suite
