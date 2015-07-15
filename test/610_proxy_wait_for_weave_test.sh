#! /bin/bash

. ./config.sh

entrypoint() {
  docker_on $HOST1 inspect --format="{{.Config.Entrypoint}}" "$@"
}

start_suite "Proxy waits for weave to be ready before running container commands"
weave_on $HOST1 launch-proxy
BASE_IMAGE=busybox
# Ensure the base image does not exist, so that it will be pulled
if (docker_on $HOST1 images $BASE_IMAGE | grep -q $BASE_IMAGE); then
  docker_on $HOST1 rmi $BASE_IMAGE
fi

assert_raises "proxy docker_on $HOST1 run --name c1 -e 'WEAVE_CIDR=10.2.1.1/24' $BASE_IMAGE $CHECK_ETHWE_UP"

# Check committed containers only have one weavewait prepended
COMMITTED_IMAGE=$(proxy docker_on $HOST1 commit c1)
assert_raises "proxy docker_on $HOST1 run --name c2 $COMMITTED_IMAGE"
assert "entrypoint c2" "$(entrypoint $COMMITTED_IMAGE)"

# Check exec works on containers without weavewait
docker_on $HOST1 run -dit --name c3 $SMALL_IMAGE /bin/sh
assert_raises "proxy docker_on $HOST1 exec c3 true"

end_suite
