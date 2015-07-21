#! /bin/bash

. ./config.sh

NAME=seetwo.weave.local

start_suite "Proxy restart reattaches networking to containers"

weave_on $HOST1 launch
proxy docker_on $HOST1 run -dt --name=c2 -h $NAME $SMALL_IMAGE /bin/sh
proxy docker_on $HOST1 run -dt --name=c1          $DNS_IMAGE   /bin/sh

C2=$(container_ip $HOST1 c2)
proxy docker_on $HOST1 restart --time=1 c2
assert_raises "proxy exec_on $HOST1 c2 $CHECK_ETHWE_UP"
assert_dns_record $HOST1 c1 $NAME $C2

end_suite
