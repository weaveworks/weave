#! /bin/bash

. ./config.sh

C1=10.2.0.78
C2=10.2.0.34
C1_NAME=c1.weave.local
C2_NAME=seetwo.weave.local

boot_containers() {
  proxy docker_on $HOST1 run -e WEAVE_CIDR=$C2/24 -dt --name=c2 -h $C2_NAME $DNS_IMAGE /bin/sh
  proxy docker_on $HOST1 run -e WEAVE_CIDR=$C1/24 -dt --name=c1             $DNS_IMAGE /bin/sh
}

kill_containers() {
  docker_on $HOST1 kill c1 c2
  docker_on $HOST1 rm   c1 c2
}

start_suite "Proxy registers containers with dns"

bridge_ip=$(weave_on $HOST1 docker-bridge-ip)

weave_on $HOST1 launch-proxy --with-dns
boot_containers
assert "exec_on $HOST1 c1 cat /etc/resolv.conf" "nameserver $bridge_ip"
assert "exec_on $HOST1 c2 cat /etc/resolv.conf" "nameserver $bridge_ip"
weave_on $HOST1 stop-proxy
kill_containers

weave_on $HOST1 launch-dns 10.2.254.1/24 $WEAVEDNS_ARGS

weave_on $HOST1 launch-proxy
boot_containers
assert_dns_record $HOST1 c1 $C2_NAME $C2
assert_dns_record $HOST1 c2 $C1_NAME $C1
weave_on $HOST1 stop-proxy
kill_containers

weave_on $HOST1 launch-proxy --without-dns
boot_containers
assert_no_dns_record $HOST1 c1 $C2_NAME
assert_no_dns_record $HOST1 c2 $C1_NAME

end_suite
