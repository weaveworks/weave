#! /bin/bash

. ./config.sh

set -e

whitely echo Ping each host from the other
run_on $HOST2 $PING $HOST1
run_on $HOST1 $PING $HOST2

whitely echo Check we can reach docker
echo
echo Host Version Info: $HOST1
echo =====================================
echo "# docker version"
docker_on $HOST1 version
echo "# docker info"
docker_on $HOST1 info
echo "# weave version"
weave_on $HOST1 version

echo
echo Host Version Info: $HOST2
echo =====================================
echo "# docker version"
docker_on $HOST2 version
echo "# docker info"
docker_on $HOST2 info
echo "# weave version"
weave_on $HOST2 version
