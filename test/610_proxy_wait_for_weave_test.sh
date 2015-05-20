#! /bin/bash

. ./config.sh

start_suite "Proxy waits for weave to be ready before running container commands"
weave_on $HOST1 launch-proxy
BASE_IMAGE=busybox
# Ensure the base image does not exist, so that it will be pulled
if (docker_on $HOST1 images $BASE_IMAGE | grep $BASE_IMAGE); then
  docker_on $HOST1 rmi $BASE_IMAGE
fi

assert_raises "proxy docker_on $HOST1 run -e 'WEAVE_CIDR=10.2.1.1/24' $BASE_IMAGE $CHECK_ETHWE_UP"

end_suite
