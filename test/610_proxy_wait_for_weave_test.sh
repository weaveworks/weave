#! /bin/bash

. ./config.sh

start_suite "Proxy waits for weave to be ready before running container commands"
weave_on $HOST1 launch-proxy
assert_raises "weave_on $HOST1 run 10.2.1.1/24 --name=c1 gliderlabs/alpine ifconfig ethwe" 0
assert "docker_on $HOST1 inspect --format='{{.Config.Entrypoint}}' c1" "[/home/weavewait/wait-for-weave]"
assert "docker_on $HOST1 inspect --format='{{.Config.Cmd}}' c1" "[ifconfig ethwe]"
end_suite

start_suite "Proxy uses entrypoint from the container with wait-for-weave"
# Setup and sanity-check
weave_on $HOST1 launch-proxy
docker_on $HOST1 build -t ifconfig-ethwe - <<- EOF
  FROM gliderlabs/alpine
  ENTRYPOINT ["ifconfig", "ethwe"]
EOF
assert "docker_on $HOST1 inspect --format='{{.Config.Entrypoint}}' ifconfig-ethwe" "[ifconfig ethwe]"
assert "docker_on $HOST1 inspect --format='{{.Config.Cmd}}' ifconfig-ethwe" "<no value>"

# Boot a new container with no entrypoint of it's own, just a command
assert_raises "weave_on $HOST1 run 10.2.1.1/24 --name=c2 ifconfig-ethwe" 0
assert "docker_on $HOST1 inspect --format='{{.Config.Entrypoint}}' c2" "[/home/weavewait/wait-for-weave]"
assert "docker_on $HOST1 inspect --format='{{.Config.Cmd}}' c2" "[ifconfig ethwe]"
end_suite
