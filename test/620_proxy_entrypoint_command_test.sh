#! /bin/bash

. ./config.sh

build_image() {
    docker_on $HOST1 build -t $1 - <<- EOF
  FROM $SMALL_IMAGE
  ENTRYPOINT $2
  CMD $3
EOF
}

start_suite "Proxy uses correct entrypoint and command with weavewait"
weave_on $HOST1 launch-proxy

build_image check-ethwe-up '["grep"]' '["^1$", "/sys/class/net/ethwe/carrier"]'
assert_raises "proxy docker_on $HOST1 run -e 'WEAVE_CIDR=10.2.1.1/24' check-ethwe-up"

build_image grep '["grep"]' ''
assert_raises "proxy docker_on $HOST1 run -e 'WEAVE_CIDR=10.2.1.1/24' grep ^1$ /sys/class/net/ethwe/carrier"

build_image false '["/bin/false"]' ''
assert_raises "proxy docker_on $HOST1 run -e 'WEAVE_CIDR=10.2.1.1/24' --entrypoint 'grep' false ^1$ /sys/class/net/ethwe/carrier"

end_suite
