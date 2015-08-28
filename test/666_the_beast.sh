#! /bin/bash

. ./config.sh

start_suite "Abuse of 'start' operation"

weave_on $HOST1 launch

proxy_start_container $HOST1 --name=c1
proxy docker_on $HOST1 create --name=c2 $SMALL_IMAGE grep "^1$" /sys/class/net/ethwe/carrier
# Now start c2 with a sneaky HostConfig
curl -X POST -H Content-Type:application/json -d '{"HostConfig": {"NetworkMode": "container:c1"}}' http://$HOST1:12375/containers/c2/start
assert "docker_on $HOST1 inspect -f '{{.State.Running}} {{.State.ExitCode}}' c2" "false 0"

end_suite
