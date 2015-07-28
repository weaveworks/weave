#! /bin/bash

. ./config.sh

start_suite "Boot the proxy should only listen on client's interface"

# Booting it over unix socket listens on unix socket
run_on $HOST1 COVERAGE=$COVERAGE sudo -E weave launch-proxy
assert_raises "run_on $HOST1 sudo docker -H unix:///var/run/weave.sock ps"
assert_raises "proxy docker_on $HOST1 ps" 1
weave_on $HOST1 stop-proxy

# Booting it over tcp listens on tcp
weave_on $HOST1 launch-proxy
assert_raises "run_on $HOST1 sudo docker -H unix:///var/run/weave.sock ps" 1
assert_raises "proxy docker_on $HOST1 ps"
weave_on $HOST1 stop-proxy

# Booting it over tcp (no prefix) listens on tcp
DOCKER_CLIENT_ARGS="-H $HOST1:$DOCKER_PORT" $WEAVE launch-proxy
assert_raises "run_on $HOST1 sudo docker -H unix:///var/run/weave.sock ps" 1
assert_raises "proxy docker_on $HOST1 ps"
weave_on $HOST1 stop-proxy


# Booting it over tls errors
assert_raises "DOCKER_CLIENT_ARGS='--tls' weave_on $HOST1 launch-proxy" 1
assert_raises "DOCKER_CERT_PATH='./tls' DOCKER_TLS_VERIFY=1 weave_on $HOST1 launch-proxy" 1

# Booting it with a specific -H overrides defaults
weave_on $HOST1 launch-proxy -H unix:///var/run/weave2.sock
assert_raises "run_on $HOST1 sudo docker -H unix:///var/run/weave2.sock ps"
assert_raises "proxy docker_on $HOST1 ps" 1
weave_on $HOST1 stop-proxy

end_suite
