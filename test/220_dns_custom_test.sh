#! /bin/bash

. ./config.sh

C1=10.2.56.34
C2=10.2.54.91
DOMAIN=foo.bar
NAME=seetwo.$DOMAIN

start_suite "Resolve names in custom domain"

weave_on $HOST1 stop || true
weave_on $HOST1 stop-dns || true
weave_on $HOST1 launch-dns 10.2.254.1/24 --domain $DOMAIN.

docker_on $HOST1 rm -f c1 c2 || true

weave_on $HOST1 run $C2/24 -t --name=c2 -h $NAME ubuntu
weave_on $HOST1 run --with-dns $C1/24 -t --name=c1 aanand/docker-dnsutils /bin/sh

assert_dns_record $HOST1 c1 $NAME $C2

end_suite
