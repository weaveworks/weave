#! /bin/bash

. ./config.sh

start_suite "Resolve a non-weave address"

weave_on $HOST1 stop-dns || true
weave_on $HOST1 launch-dns 10.2.254.1/24 -debug
docker_on $HOST1 rm -f c1 || true

weave_on $HOST1 run --with-dns 10.2.1.5/24 --name=c1 -t aanand/docker-dnsutils /bin/sh

ok=$(exec_on $HOST1 c1 host -t mx weave.works)
assert_raises "echo $ok | grep google"

ok=$(exec_on $HOST1 c1 getent hosts 8.8.8.8)
assert_raises "echo $ok | grep google"

end_suite
