#! /bin/bash

. ./config.sh

UNIVERSE=10.2.0.0/16
IPTABLES_BACKUP=/tmp/700_status_and_report_test.iptables
CHECKPOINT_URL=https://checkpoint-api.weave.works/v1/check/weave-net

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



# Block outgoing traffic to Weave's checkpoint API, to prevent retrieval of latest version:
run_on $HOST1 "sudo iptables-save > $IPTABLES_BACKUP"
[ -z "$DEBUG" ] || greyly echo "Backed iptables rules up:"
[ -z "$DEBUG" ] || run_on $HOST1 cat $IPTABLES_BACKUP
[ -z "$DEBUG" ] || greyly echo "Updating iptables rules."
run_on $HOST1 sudo iptables -A OUTPUT -p tcp -d checkpoint-api.weave.works --dport 443 -j REJECT --reject-with tcp-reset
[ -z "$DEBUG" ] || greyly echo "Updated iptables rules:"
[ -z "$DEBUG" ] || run_on $HOST1 sudo iptables -L
[ -z "$DEBUG" ] || greyly echo "Test access to checkpoint:"
[ -z "$DEBUG" ] || run_on $HOST1 "curl -X GET $CHECKPOINT_URL || true"

weave_on $HOST1 reset

CHECKPOINT_DISABLE="" weave_on $HOST1 launch

assert "weave_on $HOST1 report -f '{{.VersionCheck.Enabled}}'" "true"
assert "weave_on $HOST1 report -f '{{.VersionCheck.Success}}'" "false"
assert "weave_on $HOST1 report -f '{{.VersionCheck.NewVersion}}'" ""
assert_raises "weave_on $HOST1 status | grep 'failed to check latest version - see logs; next check at'"


[ -z "$DEBUG" ] || greyly echo "Reverting iptables rules."
run_on $HOST1 "sudo iptables-restore < $IPTABLES_BACKUP"
[ -z "$DEBUG" ] || greyly echo "Reverted iptables rules:"
[ -z "$DEBUG" ] || run_on $HOST1 sudo iptables -L
[ -z "$DEBUG" ] || greyly echo "Test access to checkpoint:"
[ -z "$DEBUG" ] || run_on $HOST1 curl -X GET $CHECKPOINT_URL



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
