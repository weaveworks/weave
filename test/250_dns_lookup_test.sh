#! /bin/bash

. ./config.sh

C1=10.2.0.78

start_suite "Cross host dns-lookup"

weave_on $HOST1 launch
weave_on $HOST2 launch $HOST1

start_container $HOST1 $C1/24 --name c1

assert "weave_on $HOST2 dns-lookup c1" $C1

end_suite
