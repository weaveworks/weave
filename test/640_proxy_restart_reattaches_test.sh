#! /bin/bash

. ./config.sh

C1=10.2.0.78
C2=10.2.0.34
NAME=seetwo.weave.local

start_suite "Proxy restart reattaches networking to containers"

weave_on $HOST1 launch-dns 10.2.254.1/24
weave_on $HOST1 launch-proxy --with-dns
docker_proxy_on $HOST1 run -e WEAVE_CIDR=$C2/24 -dt --name=c2 -h $NAME gliderlabs/alpine /bin/sh
docker_proxy_on $HOST1 run -e WEAVE_CIDR=$C1/24 -dt --name=c1 aanand/docker-dnsutils /bin/sh

docker_proxy_on $HOST1 restart c2
assert_raises "proxy_exec_on $HOST1 c2 $CHECK_ETHWE_UP"
assert_dns_record $HOST1 c1 $NAME $C2

end_suite
