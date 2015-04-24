#! /bin/bash

. ./config.sh

start_suite "Resolve a non-weave address"

weave_on $HOST1 launch-dns 10.2.254.1/24 -debug

weave_on $HOST1 run --with-dns 10.2.1.5/24 --name=c1 -t aanand/docker-dnsutils /bin/sh

assert_raises "exec_on $HOST1 c1 host -t mx weave.works | grep google"
assert_raises "exec_on $HOST1 c1 getent hosts 8.8.8.8   | grep google"

end_suite
