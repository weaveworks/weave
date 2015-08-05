#! /bin/bash
# Explicitly not called _test.sh - this isn't run, but imported by other tests.

. ./config.sh

docker_py_test() {
    SHARD=$1
    TOTAL_SHARDS=$2

    start_suite "Run docker-py test suite against the proxy"

    # Get a list of the tests for use to shard
    docker_on $HOST1 pull joffrey/docker-py >/dev/null
    C=$(docker_on $HOST1 create \
        -e NOT_ON_HOST=true \
        -e DOCKER_HOST=tcp://172.17.42.1:12375 \
        -v /tmp:/tmp \
        -v /var/run/docker.sock:/var/run/docker.sock \
        joffrey/docker-py)
    CANDIDATES=$(docker_on $HOST1 cp $C:/home/docker-py/tests/integration_test.py - | sed -En 's/^class (Test[[:alpha:]]+).*/\1/p')

    i=0
    TESTS=
    for test in $CANDIDATES; do
        if [ $(($i % $TOTAL_SHARDS)) -eq $SHARD ]; then
              TESTS="$TESTS $test"
        fi
        i=$(($i + 1))
    done

    weave_on $HOST1 launch-proxy --no-default-ipalloc

    if docker_on $HOST1 run \
        -e NOT_ON_HOST=true \
        -e DOCKER_HOST=tcp://172.17.42.1:12375 \
        -v /tmp:/tmp \
        -v /var/run/docker.sock:/var/run/docker.sock \
        joffrey/docker-py python tests/integration_test.py $TESTS ; then
        assert_raises "true"
    else
        assert_raises "false"
    fi

    end_suite
}
