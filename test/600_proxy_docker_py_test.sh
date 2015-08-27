#! /bin/bash

. ./config.sh

start_suite "Run docker-py test suite against the proxy"

docker_on $HOST1 pull joffrey/docker-py >/dev/null

weave_on $HOST1 launch-proxy --no-default-ipam

if docker_on $HOST1 run \
    -e NOT_ON_HOST=true \
    -e DOCKER_HOST=tcp://172.17.42.1:12375 \
    -v /tmp:/tmp \
    -v /var/run/docker.sock:/var/run/docker.sock \
    joffrey/docker-py py.test tests/integration_test.py ; then
    assert_raises "true"
else
    assert_raises "false"
fi

end_suite
