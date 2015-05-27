#! /bin/bash

. ./config.sh

build_image() {
    docker_on $HOST1 build -t $1 - <<- EOF
  FROM $SMALL_IMAGE
  ENTRYPOINT $2
  CMD $3
EOF
}

run_container() {
    assert_raises "proxy docker_on $HOST1 run -e 'WEAVE_CIDR=10.2.1.1/24' $1"
}

start_suite "Proxy uses correct entrypoint and command with weavewait"
weave_on $HOST1 launch-proxy

build_image check-ethwe-up '["grep"]' '["^1$", "/sys/class/net/ethwe/carrier"]'
run_container "check-ethwe-up"

build_image grep '["grep"]' ''
run_container "grep ^1$ /sys/class/net/ethwe/carrier"

build_image false '["/bin/false"]' ''
run_container "--entrypoint='grep' false ^1$ /sys/class/net/ethwe/carrier"

weave_on $HOST1 launch -iprange 10.2.2.0/24
docker_on $HOST1 kill weaveproxy
weave_on $HOST1 launch-proxy --with-ipam

assert_raises "proxy docker_on $HOST1 run check-ethwe-up"

end_suite
