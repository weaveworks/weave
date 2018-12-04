#! /bin/bash

. "$(dirname "$0")/config.sh"

C1=10.2.1.13

start_suite "Start containers with 'weave run' via proxy"

weave_on $HOST1 launch

proxy start_container $HOST1 $C1/24 --name=c1
# check we get exactly one IP back, the one specified
assert "weave_on $HOST1 ps c1 | cut -d ' ' -f 3-" $C1/24

end_suite
