#! /bin/bash

. ./config.sh

C1=10.2.56.34
C2=10.2.54.91
DOMAIN=foo.bar
NAME=seetwo.$DOMAIN

start_suite "Resolve names in custom domain"

launch_dns_on $HOST1 10.2.254.1/24 --domain $DOMAIN.

start_container          $HOST1 $C2/24 --name=c2 -h $NAME
start_container_with_dns $HOST1 $C1/24 --name=c1

assert_dns_record $HOST1 c1 $NAME $C2

end_suite
