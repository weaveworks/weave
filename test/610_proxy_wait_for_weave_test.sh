#! /bin/bash

. ./config.sh

entrypoint() {
  docker_on $HOST1 inspect --format="{{.Config.Entrypoint}}" "$@"
}

start_suite "Proxy waits for weave to be ready before running container commands"
weave_on $HOST1 launch-proxy
BASE_IMAGE=busybox
# Ensure the base image does not exist, so that it will be pulled
! docker_on $HOST1 inspect --format=" " $BASE_IMAGE >/dev/null 2>&1 || docker_on $HOST1 rmi $BASE_IMAGE

assert_raises "proxy docker_on $HOST1 run --name c1 -e 'WEAVE_CIDR=10.2.1.1/24' $BASE_IMAGE $CHECK_ETHWE_UP"

# Check committed containers only have one weavewait prepended
COMMITTED_IMAGE=$(proxy docker_on $HOST1 commit c1)
assert_raises "proxy docker_on $HOST1 run --name c2 $COMMITTED_IMAGE"
assert "entrypoint c2" "$(entrypoint $COMMITTED_IMAGE)"

end_suite
