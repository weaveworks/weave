#! /bin/bash

. ./config.sh

C1=10.2.0.78
C2=10.2.0.34
C3=10.2.0.57
CNAME1=qiuds71y827hdi-seeone-1io9qd9i0wd
NAME1=seeone.weave.local
CNAME2=124DJKSNK812-seetwo-128hbaJ881
NAME2=seetwo.weave.local
CNAME3=doesnotmatchpattern
NAME3=doesnotmatchpattern.weave.local

start_suite "Hostname derivation through container name substitutions"

weave_on $HOST1 launch-router
weave_on $HOST1 launch-proxy --hostname-match '^[^-]+-(?P<appname>[^-]*)-[^-]+$' --hostname-replacement '$appname'

proxy_start_container_with_dns $HOST1 -e WEAVE_CIDR=$C1/24 --name=$CNAME1
proxy_start_container_with_dns $HOST1 -e WEAVE_CIDR=$C2/24 --name=$CNAME2
proxy_start_container_with_dns $HOST1 -e WEAVE_CIDR=$C3/24 --name=$CNAME3

assert_dns_a_record $HOST1 $CNAME1 $NAME2 $C2
assert_dns_a_record $HOST1 $CNAME2 $NAME3 $C3
assert_dns_a_record $HOST1 $CNAME3 $NAME1 $C1

end_suite
