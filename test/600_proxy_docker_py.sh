#! /bin/bash
# Explicitly not called _test.sh - this isn't run, but imported by other tests.

. ./config.sh

docker_py_test() {
    SHARD=$1
    TOTAL_SHARDS=$2

    start_suite "Run docker-py test suite against the proxy"

    # Get a list of the tests for use to shard
    docker_on $HOST1 pull joffrey/docker-py >/dev/null
    CANDIDATES=$(docker_on $HOST1 run \
      joffrey/docker-py \
      py.test --collect-only tests/integration_test.py \
      | sed -En "s/\s*<UnitTestCase '([[:alpha:]]+)'>/\1/p")

    i=0
    TESTS=
    for test in $CANDIDATES; do
        if [ $(($i % $TOTAL_SHARDS)) -eq $SHARD ]; then
              TESTS="$TESTS tests/integration_test.py::$test"
        fi
        i=$(($i + 1))
    done

    weave_on $HOST1 launch-proxy --no-default-ipalloc

    DOCKER_BRIDGE_IP=$(weave_on $HOST1 docker-bridge-ip)

    if docker_on $HOST1 run \
        -e NOT_ON_HOST=true \
        -e DOCKER_HOST=tcp://$DOCKER_BRIDGE_IP:12375 \
        -v /tmp:/tmp \
        -v /var/run/docker.sock:/var/run/docker.sock \
        joffrey/docker-py py.test $TESTS ; then
        assert_raises "true"
    else
        assert_raises "false"
    fi

    end_suite
}
