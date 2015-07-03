#! /bin/bash

. ./config.sh

start_suite "Configure the docker daemon for the proxy"

weave_on $HOST1 launch-proxy

CMD="run -e WEAVE_CIDR=10.2.1.4/24 $SMALL_IMAGE $CHECK_ETHWE_UP"
assert_raises "eval '$(weave_on $HOST1 proxy-env)' ; docker $CMD"
assert_raises "docker $(weave_on $HOST1 proxy-config) $CMD"

# Check we can use the weave script through the proxy
assert_raises "eval '$(weave_on $HOST1 proxy-env)' ; $WEAVE version"
assert_raises "eval '$(weave_on $HOST1 proxy-env)' ; $WEAVE ps"
assert_raises "eval '$(weave_on $HOST1 proxy-env)' ; $WEAVE launch-router"

end_suite
