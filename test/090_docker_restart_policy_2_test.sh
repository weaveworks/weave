#! /bin/bash

. ./config.sh

start_suite "Test docker restart policy"

weave_on $HOST1 launch

assert "docker_on $HOST1 inspect -f '{{.HostConfig.RestartPolicy.Name}}' weave weaveproxy weaveplugin" "always\nalways\nalways"

assert_raises "check_restart $HOST1 weave"
assert_raises "check_restart $HOST1 weaveproxy"
assert_raises "check_restart $HOST1 weaveplugin"

# stop + launch tests that restart policy changes result
# in the old containers being removed and new ones created
weave_on $HOST1 stop
weave_on $HOST1 launch --no-restart

assert "docker_on $HOST1 inspect -f '{{.HostConfig.RestartPolicy.Name}}' weave weaveproxy weaveplugin" "no\nno\nno"

assert_raises "! check_restart $HOST1 weave"
assert_raises "! check_restart $HOST1 weaveproxy"
assert_raises "! check_restart $HOST1 weaveplugin"

# Relaunch the plugin to prevent the `weave stop` in `end_suite`
# timing out trying to remove the plugin network
weave_on $HOST1 launch-plugin

end_suite
