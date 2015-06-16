#! /bin/bash

. ./config.sh

C1=10.2.0.78
C2=10.2.0.34
C3=10.2.0.57
DOMAIN=weave.local
NAME=seeone.$DOMAIN

start_suite "Resolve unqualified names"

weave_on $HOST1 launch-dns 10.2.254.1/24 $WEAVEDNS_ARGS

start_container          $HOST1 $C1/24 --name=c1 -h $NAME
start_container_with_dns $HOST1 $C2/24 --name=c2 -h seetwo.$DOMAIN
start_container_with_dns $HOST1 $C3/24 --name=c3 --dns-search=$DOMAIN

assert "exec_on $HOST1 c2 getent hosts seeone | tr -s ' '" "$C1 $NAME"
assert "exec_on $HOST1 c3 getent hosts seeone | tr -s ' '" "$C1 $NAME"

end_suite
