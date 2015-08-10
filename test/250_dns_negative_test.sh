#! /bin/bash

. ./config.sh

start_suite "Negative DNS queries"

weave_on $HOST1 launch
start_container_with_dns $HOST1 --name c1
assert_raises "exec_on $HOST1 c1 dig A foo.weave.local | grep 'status: NXDOMAIN'"
assert_raises "exec_on $HOST1 c1 dig A foo.invalid | grep 'status: NXDOMAIN'"

end_suite
