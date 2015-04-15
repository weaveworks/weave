#! /bin/bash

. ./config.sh

UNIVERSE=10.2.3.0/24

start_suite "Resolve names over cross-host weave network with IPAM"

weave_on $HOST1 launch -iprange $UNIVERSE
weave_on $HOST2 launch -iprange $UNIVERSE $HOST1

weave_on $HOST1 launch-dns 10.2.254.1/24
weave_on $HOST2 launch-dns 10.2.254.2/24

weave_on $HOST2 run -t --name=c2 -h seetwo.weave.local gliderlabs/alpine /bin/sh
weave_on $HOST1 run --with-dns --name=c1 -t aanand/docker-dnsutils /bin/sh

# Note can't use weave_on here because it echoes the command
C2IP=$(DOCKER_HOST=tcp://$HOST2:2375 $WEAVE ps c2 | grep -o -E '[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}')

ok=$(docker -H tcp://$HOST1:2375 exec -i c1 dig +short seetwo.weave.local)
assert "echo $ok" "$C2IP"

ok=$(docker -H tcp://$HOST1:2375 exec -i c1 dig +short -x $C2IP)
assert "echo $ok" "seetwo.weave.local."

end_suite
