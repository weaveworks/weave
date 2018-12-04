#! /bin/bash

. "$(dirname "$0")/config.sh"

C1=10.2.0.78
C2=10.2.0.79
C3=10.2.0.80

start_suite "DNS lookup case (in)sensitivity"

weave_on $HOST1 launch

start_container_with_dns $HOST1 --name=test

start_container $HOST1 $C1/24 --name=seeone
assert_dns_record $HOST1 test seeone.weave.local $C1
assert_dns_record $HOST1 test SeeOne.weave.local $C1
assert_dns_record $HOST1 test SEEONE.weave.local $C1

start_container $HOST1 $C2/24 --name=SeEtWo
assert_dns_record $HOST1 test seetwo.weave.local $C2
assert_dns_record $HOST1 test SeeTwo.weave.local $C2
assert_dns_record $HOST1 test SEETWO.weave.local $C2

start_container $HOST1 $C3/24 --name=seetwo
assert_dns_record $HOST1 test seetwo.weave.local $C2 $C3
assert_dns_record $HOST1 test SeeTwo.weave.local $C2 $C3
assert_dns_record $HOST1 test SEETWO.weave.local $C2 $C3
assert "exec_on $HOST1 test dig +short seetwo.weave.local A | grep -v ';;' | wc -l" 2

end_suite
