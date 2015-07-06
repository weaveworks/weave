#! /bin/bash

. ./config.sh

start_suite "Configure the docker daemon for the proxy"

# No output when nothing running
assert "weave_on $HOST1 env" ""
assert "weave_on $HOST1 config" ""

weave_on $HOST1 launch-proxy

CMD="run -e WEAVE_CIDR=10.2.1.4/24 $SMALL_IMAGE $CHECK_ETHWE_UP"
assert_raises "eval '$(weave_on $HOST1 env)' ; docker $CMD"
assert_raises "docker $(weave_on $HOST1 config) $CMD"

# Check we can use the weave script through the proxy
assert_raises "eval '$(weave_on $HOST1 env)' ; $WEAVE version"
assert_raises "eval '$(weave_on $HOST1 env)' ; $WEAVE ps"
assert_raises "eval '$(weave_on $HOST1 env)' ; $WEAVE launch-router"

end_suite
