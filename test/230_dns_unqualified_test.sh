#! /bin/bash

. ./config.sh

C1=10.2.0.78
C2=10.2.0.34
C3=10.2.0.57
DOMAIN=weave.local
NAME=seeone.$DOMAIN

start_suite "Resolve unqualified names"

weave_on $HOST1 launch-dns 10.2.254.1/24

weave_on $HOST1 run $C1/24 -t --name=c1 -h $NAME ubuntu
weave_on $HOST1 run --with-dns $C2/24 -t --name=c2 -h seetwo.$DOMAIN aanand/docker-dnsutils /bin/sh
weave_on $HOST1 run --with-dns $C3/24 -t --name=c3 --dns-search=weave.local aanand/docker-dnsutils /bin/sh

assert "exec_on $HOST1 c2 getent hosts seeone | tr -s ' '" "$C1 $NAME"
assert "exec_on $HOST1 c3 getent hosts seeone | tr -s ' '" "$C1 $NAME"

end_suite
