#! /bin/bash

. ./config.sh

start_suite "weave status/weave report"

weave_on $HOST1 launch --nickname nicknamington

start_container $HOST1 --name test

# Assert report template $2 has value $3 on host $1
assert_report() {
    assert "weave_on $1 report -f '$2'" "$3"
}

# Assert status property $2 has value $3 on host $1
assert_status() {
    assert "weave_on $1 status | grep -oP '(?<= $2: ).*'" "$3"
}

assert_report $HOST1 "{{.Router.NickName}}"                       "nicknamington"
assert_report $HOST1 "{{len .Router.Peers}}"                      "1"
assert_report $HOST1 "{{.IPAM.Range}}"                            "[10.32.0.0-10.48.0.0)"
assert_report $HOST1 "{{.IPAM.DefaultSubnet}}"                    "10.32.0.0/12"
assert_report $HOST1 "{{.DNS.Domain}}"                            "weave.local."
assert_report $HOST1 "{{range .DNS.Entries}}{{.Hostname}}{{end}}" "test.weave.local."

assert_status $HOST1 "NickName"      "nicknamington"
assert_status $HOST1 "Peers"         "1"
assert_status $HOST1 "Range"         "[10.32.0.0-10.48.0.0)"
assert_status $HOST1 "DefaultSubnet" "10.32.0.0/12"
assert_status $HOST1 "Domain"        "weave.local."
assert "weave_on $HOST1 status dns | tr -s ' ' | cut -d ' ' -f 1" "test"

end_suite
