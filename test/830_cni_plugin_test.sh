#! /bin/bash

. ./config.sh

start_suite "Test CNI plugin"

cni_connect() {
    pid=$(container_pid $1 $2)
    id=$(docker_on $1 inspect -f '{{.Id}}' $2)
    run_on $1 CNI_VERSION=1 CNI_COMMAND=ADD CNI_CONTAINERID=$id CNI_IFNAME=eth0 \
    CNI_NETNS=/proc/$pid/ns/net CNI_PATH=/opt/cni/bin /opt/cni/bin/weave-net 
}

run_on $HOST1 sudo mkdir -p /opt/cni/bin
# setup-cni is a subset of 'weave setup', without doing any 'docker pull's
weave_on $HOST1 setup-cni
weave_on $HOST1 launch
weave_on $HOST1 expose

C1=$(docker_on $HOST1 run --net=none --name=c1 -dt $SMALL_IMAGE /bin/sh)
C2=$(docker_on $HOST1 run --net=none --name=c2 -dt $SMALL_IMAGE /bin/sh)

cni_connect $HOST1 c1 <<EOF
{
    "name": "weave",
    "type": "weave-net"
}
EOF
cni_connect $HOST1 c2 <<EOF
{
    "name": "weave",
    "type": "weave-net",
    "ipam": {
        "type": "weave-ipam",
        "routes": [ { "dst": "10.32.0.0/12" } ]
    }
}
EOF

C1IP=$(container_ip $HOST1 c1)
C2IP=$(container_ip $HOST1 c2)

assert_raises "exec_on $HOST1 c1 $PING $C2IP"
assert_raises "exec_on $HOST1 c2 $PING $C1IP"

# Now remove and start a new container to see if IP address re-use breaks things
docker_on $HOST1 rm -f c2
sleep 1

docker_on $HOST1 run --net=none --name=c3 -dt $SMALL_IMAGE /bin/sh

cni_connect $HOST1 c3 <<EOF
{ "name": "weave", "type": "weave-net" }
EOF

assert_raises "exec_on $HOST1 c1 $PING $C2IP"

end_suite
