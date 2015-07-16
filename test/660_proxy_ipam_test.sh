#! /bin/bash

. ./config.sh

UNIVERSE=10.2.2.0/24

start() {
  host=$1
  shift
  proxy docker_on "$host" run "$@" -dt $SMALL_IMAGE /bin/sh
}

assert_no_ethwe() {
  assert_raises "container_ip $1 $2" 1
  assert_raises "proxy exec_on $1 $2 ip link show | grep -v ethwe"
}

start_suite "Ping proxied containers over cross-host weave network (with IPAM)"

weave_on $HOST1 launch-router --ipalloc-range $UNIVERSE
weave_on $HOST2 launch-router --ipalloc-range $UNIVERSE $HOST1
weave_on $HOST1 launch-proxy
weave_on $HOST2 launch-proxy --no-default-ipalloc

start $HOST1 --name=auto
start $HOST1 --name=none       -e WEAVE_CIDR=none
start $HOST2 --name=zero       -e WEAVE_CIDR=
start $HOST2 --name=no-default
start $HOST1 --name=bridge     --net=bridge
start $HOST1 --name=host       --net=host
start $HOST1 --name=other      --net=container:auto

AUTO=$(container_ip $HOST1 auto)
ZERO=$(container_ip $HOST2 zero)
BRIDGE=$(container_ip $HOST1 bridge)
OTHER=$(container_ip $HOST1 other)
assert_raises "proxy exec_on $HOST1 auto $PING $ZERO"
assert_raises "proxy exec_on $HOST2 zero $PING $AUTO"
assert_raises "proxy exec_on $HOST2 zero $PING $BRIDGE"
assert_raises "proxy exec_on $HOST2 zero $PING $OTHER"

assert_no_ethwe $HOST1 none
assert_no_ethwe $HOST2 no-default
assert_no_ethwe $HOST1 host

end_suite
