#! /bin/bash

. ./config.sh

C1=10.2.0.78
C2=10.2.0.34
NAME=seetwo.weave.local

start_suite "Proxy restart reattaches networking to containers"

weave_on $HOST1 launch
proxy docker_on $HOST1 run -e WEAVE_CIDR=$C2/24 -dt --name=c2 -h $NAME $SMALL_IMAGE /bin/sh
proxy docker_on $HOST1 run -e WEAVE_CIDR=$C1/24 -dt --name=c1          $DNS_IMAGE   /bin/sh

proxy docker_on $HOST1 restart c2
assert_raises "proxy exec_on $HOST1 c2 $CHECK_ETHWE_UP"
assert_dns_record $HOST1 c1 $NAME $C2

end_suite
