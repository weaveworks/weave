#!/bin/bash

# TODO(mp) 1. test anti-replay 2. test sleeve <-> fastdp
# TODO(mp) test with --trusted-subnets

. "$(dirname "$0")/config.sh"

HOST1_IP=$($SSH $HOST1 getent ahosts $HOST1 | grep "RAW" | cut -d' ' -f1)
HOST2_IP=$($SSH $HOST2 getent ahosts $HOST2 | grep "RAW" | cut -d' ' -f1)

start_tcpdump() {
    HOST=$1
    PCAP=$2
    run_on $HOST "sudo nohup tcpdump -i any -w $PCAP >/dev/null 2>&1 & echo \$! > $PCAP.pid"
}

stop_tcpdump() {
    HOST=$1
    PCAP=$2
    $SSH $HOST "sudo kill \$(cat $PCAP.pid) && rm -f $PCAP.pid && sudo base64 $PCAP" \
        | base64 -d > $PCAP
}

# Checks whether all vxlan-tunneled traffic is encrypted.
assert_pcap() {
    PCAP=$1
    # UDP port is not exposed in ESP
    assert "tcpdump -r $PCAP 'src $HOST1_IP && dst $HOST2_IP && dst port 6784'" ""
    assert "tcpdump -r $PCAP 'src $HOST2_IP && dst $HOST1_IP && dst port 6784'" ""
    # ip_proto(ESP) = 50
    assert_raises "[[ -n \"$(tcpdump -r $PCAP "src $HOST1_IP && dst $HOST2_IP && proto 50")\" ]]"
    assert_raises "[[ -n \"$(tcpdump -r $PCAP "src $HOST2_IP && dst $HOST1_IP && proto 50")\" ]]"

    rm -f $PCAP
}

C1=10.2.1.4
C2=10.2.1.7

start_suite "Ping over encrypted cross-host weave network (fastdp)"

weave_on $HOST1 launch --password wfvAwt7sj
weave_on $HOST2 launch --password wfvAwt7sj $HOST1

assert_raises "weave_on $HOST1 status connections | grep -P 'encrypted *fastdp'"
assert_raises "weave_on $HOST2 status connections | grep -P 'encrypted *fastdp'"

PCAP1=$(mktemp)
PCAP2=$(mktemp)

start_tcpdump $HOST1 $PCAP1
start_tcpdump $HOST2 $PCAP2

start_container $HOST1 $C1/24 --name=c1
start_container $HOST2 $C2/24 --name=c2
assert_raises "exec_on $HOST1 c1 $PING $C2"

stop_tcpdump $HOST1 $PCAP1
stop_tcpdump $HOST2 $PCAP2

assert_pcap $PCAP1
assert_pcap $PCAP2

weave_on $HOST2 reset
# policies/SAs and iptables rules should have been removed after terminating
# the connection.
assert "$SSH $HOST1 sudo ip xfrm state" ""
assert "$SSH $HOST1 sudo ip xfrm policy" ""
assert "$SSH $HOST1 sudo iptables -t mangle -S WEAVE-IPSEC | grep '\-A'" ""

end_suite
