#! /bin/bash

. "$(dirname "$0")/config.sh"

start_suite "setup pulls images"

weave_on $HOST1 setup

start_container $HOST1 --name=c1

# And try to start a container
assert_raises "timeout 10 cat <( start_container $HOST1 )"

end_suite
