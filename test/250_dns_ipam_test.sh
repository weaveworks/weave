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
C2=$(weave_on $HOST2 ps c2 | grep -o -E '[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}')

assert "exec_on $HOST1 c1 dig +short seetwo.weave.local" "$C2"
assert "exec_on $HOST1 c1 dig +short -x $C2" "seetwo.weave.local."

end_suite
