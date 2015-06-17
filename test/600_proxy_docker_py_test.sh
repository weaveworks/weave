#! /bin/bash

. ./config.sh

start_suite "Run docker-py test suite against the proxy"

weave_on $HOST1 launch-proxy --no-default-ipam
assert_raises "docker_on $HOST1 ps | grep weaveproxy"
assert_raises "docker_on $HOST1 run \
  -e NOT_ON_HOST=true \
  -e DOCKER_HOST=tcp://172.17.42.1:12375 \
  -v /tmp:/tmp \
  -v /var/run/docker.sock:/var/run/docker.sock \
  joffrey/docker-py python tests/integration_test.py"

end_suite
