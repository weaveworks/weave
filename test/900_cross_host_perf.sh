#! /bin/bash

. ./config.sh

[ -n "$PERFORMANCE" ] || exit 0

C1=10.2.2.4
C2=10.2.2.7
UNIVERSE=10.2.0.0/16

start_perf_suite "Iperf over cross-host weave network"

weave_on $HOST1 launch-router --ipalloc-range $UNIVERSE 
weave_on $HOST2 launch-router --ipalloc-range $UNIVERSE $HOST1

start_iperf_server $HOST1 ip:$C1/24
start_iperf_client $HOST2 ip:$C2/24 "iperf_cross" -t10

end_perf_suite
