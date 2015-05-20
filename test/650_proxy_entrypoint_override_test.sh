#! /bin/bash

. ./config.sh

start_suite "Proxy allows overridden entrypoint from the container with weavewait"
weave_on $HOST1 launch-proxy
docker_on $HOST1 build -t false - <<- EOF
  FROM gliderlabs/alpine
  ENTRYPOINT ["/bin/false"]
EOF

assert_raises "proxy docker_on $HOST1 run -e 'WEAVE_CIDR=10.2.1.1/24' --entrypoint '/sbin/ip' false link show ethwe | grep 'state UP'"

end_suite
