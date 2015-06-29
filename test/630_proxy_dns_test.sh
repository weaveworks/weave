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
  rm_containers $HOST1 c1 c2
}

assert_no_resolution() {
  boot_containers
  assert_no_dns_record $HOST1 c1 $C2_NAME $C2
  assert_no_dns_record $HOST1 c2 $C1_NAME $C1
  kill_containers
}

start_suite "Proxy registers containers with dns"

bridge_ip=$(weave_on $HOST1 docker-bridge-ip)

# Assert behaviour without weaveDNS running, but dns forced
weave_on $HOST1 launch-proxy --with-dns
assert_no_resolution
weave_on $HOST1 stop-proxy

# Assert behaviour without weaveDNS
weave_on $HOST1 launch-proxy
assert_no_resolution

# Assert behaviour with weaveDNS running
weave_on $HOST1 launch-router
boot_containers
assert_dns_record $HOST1 c1 $C2_NAME $C2
assert_dns_record $HOST1 c2 $C1_NAME $C1
kill_containers
weave_on $HOST1 stop-proxy

# Assert behaviour with weaveDNS running, but dns forced off
weave_on $HOST1 launch-proxy --without-dns
assert_no_resolution

end_suite
