#! /bin/bash

. ./config.sh

build_image() {
    docker_on $1 build -t $2 - <<- EOF
  FROM $SMALL_IMAGE
  ENTRYPOINT $3
  CMD $4
EOF
}

run_container() {
    assert_raises "proxy docker_on $1 run -e 'WEAVE_CIDR=10.2.1.1/24' $2"
}

start_suite "Proxy uses correct entrypoint and command with weavewait"
weave_on $HOST1 launch-proxy

build_image $HOST1 check-ethwe-up '["grep"]' '["^1$", "/sys/class/net/ethwe/carrier"]'
run_container $HOST1 "check-ethwe-up"

build_image $HOST1 grep '["grep"]' ''
run_container $HOST1 "grep ^1$ /sys/class/net/ethwe/carrier"

build_image $HOST1 false '["/bin/false"]' ''
run_container $HOST1 "--entrypoint='grep' false ^1$ /sys/class/net/ethwe/carrier"

weave_on $HOST2 launch -iprange 10.2.2.0/24
weave_on $HOST2 launch-proxy --with-ipam

build_image $HOST2 false '["/bin/false"]' ''
assert_raises "proxy docker_on $HOST2 run --entrypoint='grep' false ^1$ /sys/class/net/ethwe/carrier"

end_suite
