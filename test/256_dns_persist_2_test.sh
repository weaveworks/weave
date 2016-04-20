#! /bin/bash

. ./config.sh

C1=10.2.0.78
C2=10.2.0.34
NAME=seetwo.weave.local
NAME1=seetwoextra.weave.local

start_suite "Check persisted DNS records"

weave_on $HOST1 launch
weave_on $HOST2 launch $HOST1

CMD="docker run --restart always --name c2 -h $NAME -e WEAVE_CIDR=$C2/24 -td $SMALL_IMAGE /bin/sh"
run_on $HOST1 "eval \$(weave env) ; $CMD"
start_container_with_dns $HOST1 $C1/24 --name=c1
start_container_with_dns $HOST2 --name=c3
weave_on $HOST1 dns-add c2 -h $NAME1

assert_dns_a_record $HOST1 c1 $NAME $C2
assert_dns_a_record $HOST1 c1 $NAME1 $C2
assert_dns_a_record $HOST2 c3 $NAME $C2
assert_dns_a_record $HOST2 c3 $NAME1 $C2

# Stop weave on $HOST1, so that c1 and c2 DNS entries get tombstoned.
weave_on $HOST1 stop

sleep 1
# $NAME and $NAME1 should be gone because of the termination.
assert_no_dns_record $HOST2 c3 $NAME
assert_no_dns_record $HOST2 c3 $NAME1

# Start weave on $HOST1; it should restore DNS entries.
weave_on $HOST1 launch

sleep 1
# $NAME and $NAME1 should be restored
assert_dns_a_record $HOST2 c3 $NAME $C2
assert_dns_a_record $HOST2 c3 $NAME1 $C2

run_on $HOST1 "eval \$(weave env) ; docker restart c2"

sleep 1
assert_dns_a_record $HOST1 c1 $NAME $C2
assert_dns_a_record $HOST1 c1 $NAME1 $C2
assert_dns_a_record $HOST2 c3 $NAME $C2
assert_dns_a_record $HOST2 c3 $NAME1 $C2

# Restart Docker on $HOST1; DNS entries of c2 should be restored
restart_docker $HOST1

sleep 5
assert_dns_a_record $HOST2 c3 $NAME $C2
assert_dns_a_record $HOST2 c3 $NAME1 $C2

end_suite
