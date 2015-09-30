#! /bin/bash

. ./config.sh

start_suite "Negative DNS queries"

weave_on $HOST1 launch
start_container_with_dns $HOST1 --name c1

# unknown name, invalid domain, and unsupported query types should all
# trigger NXDOMAIN
assert_raises "exec_on $HOST1 c1 dig A foo.weave.local | grep -q 'status: NXDOMAIN'"
assert_raises "exec_on $HOST1 c1 dig A foo.invalid     | grep -q 'status: NXDOMAIN'"
assert_raises "exec_on $HOST1 c1 dig MX $NAME          | grep -q 'status: NXDOMAIN'"

end_suite
