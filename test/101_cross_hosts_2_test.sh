#! /bin/bash

. "$(dirname "$0")/config.sh"

C1=10.32.1.4
C2=10.32.1.7
IMAGE=weaveworks/network-tester

network_tester_status() {
    # we only need to contact one container as it checks contact in both directions
    status=$($SSH $HOST1 curl -sS http://127.0.0.1:8080/status)
    [ -n "$status" -a "$status" != "running" ]
}

established_metric() {
    $SSH $HOST1 curl -sS http://127.0.0.1:6784/metrics | awk -e '/^weave_connections.*state="established"/ { print $2 }'
}

start_suite "Network test over cross-host weave network"

weave_on $HOST1 launch
weave_on $HOST2 launch $HOST1

docker_on $HOST1 run --name=c1 -dt -p 8080:8080 $IMAGE -peers=2 --iface=ethwe $C1 $C2
weave_on  $HOST1 attach $C1/24 c1
docker_on $HOST2 run --name=c2 -dt -p 8080:8080 $IMAGE -peers=2 --iface=ethwe $C1 $C2
weave_on  $HOST2 attach $C2/24 c2

wait_for_x network_tester_status "network tester status"
assert "echo $status" "pass"
assert "established_metric" "1"

end_suite
