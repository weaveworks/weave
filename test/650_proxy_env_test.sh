#! /bin/bash

. ./config.sh

start_suite "Configure the docker daemon for the proxy"

weave_on $HOST1 launch-proxy

CMD="run -e WEAVE_CIDR=10.2.1.4/24 $SMALL_IMAGE $CHECK_ETHWE_UP"
assert_raises "eval '$(weave_on $HOST1 proxy-env)' ; docker $CMD"
assert_raises "docker $(weave_on $HOST1 proxy-config) $CMD"

end_suite
