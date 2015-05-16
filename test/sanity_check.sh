#! /bin/bash

. ./config.sh

set -e

whitely echo Ping each host from the other
run_on $HOST2 $PING $HOST1
run_on $HOST1 $PING $HOST2

whitely echo Check we can reach docker
docker_on $HOST1 info
docker_on $HOST2 info
