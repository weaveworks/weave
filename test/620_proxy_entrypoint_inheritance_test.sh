#! /bin/bash

. ./config.sh

start_suite "Proxy uses correct entrypoint and command with weavewait"
weave_on $HOST1 launch-proxy

docker_on $HOST1 build -t check-ethwe-up - <<- EOF
  FROM $SMALL_IMAGE
  ENTRYPOINT ["grep"]
  CMD ["^1$", "/sys/class/net/ethwe/carrier"]
EOF
assert_raises "proxy docker_on $HOST1 run -e 'WEAVE_CIDR=10.2.1.1/24' check-ethwe-up"

docker_on $HOST1 build -t grep - <<- EOF
  FROM $SMALL_IMAGE
  ENTRYPOINT ["grep"]
EOF
assert_raises "proxy docker_on $HOST1 run -e 'WEAVE_CIDR=10.2.1.1/24' grep ^1$ /sys/class/net/ethwe/carrier"

docker_on $HOST1 build -t false - <<- EOF
  FROM $SMALL_IMAGE
  ENTRYPOINT ["/bin/false"]
EOF
assert_raises "proxy docker_on $HOST1 run -e 'WEAVE_CIDR=10.2.1.1/24' --entrypoint 'grep' false ^1$ /sys/class/net/ethwe/carrier"

end_suite
