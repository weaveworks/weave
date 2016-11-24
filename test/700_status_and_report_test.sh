#! /bin/bash

. ./config.sh

UNIVERSE=10.2.0.0/16

start_suite "weave status/report"



weave_on $HOST1 launch --ipalloc-range $UNIVERSE --name 8a:3e:3e:3e:3e:3e --nickname nicknamington

check() {
    assert "weave_on $HOST1 status | grep -oP '(?<= $1: ).*'" "$3"
    assert "weave_on $HOST1 report -f '$2'" "$3"
}

check "Name"          "{{.Router.Name}}({{.Router.NickName}})" "8a:3e:3e:3e:3e:3e(nicknamington)"
check "Peers"         "{{len .Router.Peers}}"                  "1"
check "DefaultSubnet" "{{.IPAM.DefaultSubnet}}"                $UNIVERSE
check "Domain"        "{{.DNS.Domain}}"                        "weave.local."

assert_raises "weave_on $HOST1 status peers | grep nicknamington"
start_container $HOST1 --name test
assert "weave_on $HOST1 status dns         | tr -s ' ' | cut -d ' ' -f 1" "test"
assert_raises "weave_on $HOST1 report | grep nicknamington"
weave_on $HOST1 connect 10.2.2.1
assert "weave_on $HOST1 status targets" "10.2.2.1"
assert "weave_on $HOST1 status connections | tr -s ' ' | cut -d ' ' -f 2" "10.2.2.1:6783"

assert "weave_on $HOST1 report -f '{{.VersionCheck.Enabled}}'" "false"
assert_raises "weave_on $HOST1 status | grep 'version check update disabled'"



weave_on $HOST1 reset
# Guarantee the version check fails by feeding an unresponsive IP address into the resolver
$SSH $HOST1 tee /tmp/hosts.checkpoint-api > /dev/null <<EOF
127.0.0.1 localhost
127.1.1.1 checkpoint-api.weave.works
EOF
CHECKPOINT_DISABLE="" WEAVE_DOCKER_ARGS="-v /tmp/hosts.checkpoint-api:/etc/hosts" weave_on $HOST1 launch

wait_for_version_check_error() {
    while true; do
        docker_on $HOST1 logs weave 2>&1 | grep 'Error checking version' && return
        sleep 1
    done
}
assert_raises "timeout 30 cat <( wait_for_version_check_error )"

assert "weave_on $HOST1 report -f '{{.VersionCheck.Enabled}}'" "true"
assert "weave_on $HOST1 report -f '{{.VersionCheck.Success}}'" "false"
assert "weave_on $HOST1 report -f '{{.VersionCheck.NewVersion}}'" ""

weave_on $HOST1 reset

CHECKPOINT_DISABLE="" weave_on $HOST1 launch
assert "weave_on $HOST1 report -f '{{.VersionCheck.Enabled}}'" "true"
assert "weave_on $HOST1 report -f '{{.VersionCheck.Success}}'" "true"

NEW_VSN=$(weave_on $HOST1 report  -f "{{.VersionCheck.NewVersion}}")
if [ -z "$NEW_VSN" ]; then
    assert_raises "weave_on $HOST1 status | grep 'up to date; next check at '"
else
    assert_raises "weave_on $HOST1 status | grep \"version $NEW_VSN available - please upgrade!\""
fi



end_suite
