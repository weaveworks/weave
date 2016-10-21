#!/bin/sh

set -e

# Default if not supplied - same as weave net default
IPALLOC_RANGE=${IPALLOC_RANGE:-10.32.0.0/12}
HTTP_ADDR=${WEAVE_HTTP_ADDR:-127.0.0.1:6784}

# kube-proxy requires that bridged traffic passes through netfilter
if [ ! -f /proc/sys/net/bridge/bridge-nf-call-iptables ] ; then
    echo /proc/sys/net/bridge/bridge-nf-call-iptables not found >&2
    exit 1
fi

echo 1 > /proc/sys/net/bridge/bridge-nf-call-iptables

# Create CNI config, if not already there
if [ ! -f /etc/cni/net.d/10-weave.conf ] ; then
    mkdir -p /etc/cni/net.d
    cat > /etc/cni/net.d/10-weave.conf <<EOF
{
    "name": "weave",
    "type": "weave-net"
}
EOF
fi

install_cni_plugin() {
    mkdir -p $1 || return 1
    cp /home/weave/plugin $1/weave-net
    cp /home/weave/plugin $1/weave-ipam
}

# Install CNI plugin binary to typical CNI bin location
# with fall-back to CNI directory used by kube-up on GCI OS
if [ ! -f /opt/cni/bin/weave-net ] ; then
    if ! install_cni_plugin /opt/cni/bin ; then
        install_cni_plugin /host_home/kubernetes/bin
    fi
fi

# Need to create bridge before running weaver so we can use the peer address
# (because of https://github.com/weaveworks/weave/issues/2480)
/home/weave/weave --local create-bridge --force --expect-npc

# Kubernetes sets HOSTNAME to the host's hostname
# when running a pod in host namespace.
NICKNAME_ARG=""
if [ -n "$HOSTNAME" ] ; then
    NICKNAME_ARG="--nickname=$HOSTNAME"
fi

BRIDGE_OPTIONS="--datapath=datapath"
if [ "$(/home/weave/weave --local bridge-type)" = "bridge" ] ; then
    # TODO: Call into weave script to do this
    if ! ip link show vethwe-pcap >/dev/null 2>&1 ; then
        ip link add name vethwe-bridge type veth peer name vethwe-pcap
        ip link set vethwe-bridge up
        ip link set vethwe-pcap up
        ip link set vethwe-bridge master weave
    fi
    BRIDGE_OPTIONS="--iface=vethwe-pcap"
fi

if [ -z "$KUBE_PEERS" ]; then
    if ! KUBE_PEERS=$(/home/weave/kube-peers) || [ -z "$KUBE_PEERS" ]; then
        echo Failed to get peers >&2
        exit 1
    fi
fi

peer_count() {
    echo $#
}

if [ -z "$IPALLOC_INIT" ]; then
    IPALLOC_INIT="consensus=$(peer_count $KUBE_PEERS)"
fi

/home/weave/weaver --port=6783 $BRIDGE_OPTIONS \
     --http-addr=127.0.0.1:6784 --docker-api='' --no-dns \
     --ipalloc-range=$IPALLOC_RANGE $NICKNAME_ARG \
     --ipalloc-init $IPALLOC_INIT \
     --name=$(cat /sys/class/net/weave/address) "$@" \
     $KUBE_PEERS &
WEAVE_PID=$!

# Wait for weave process to become responsive
while true ; do
    curl $HTTP_ADDR/status >/dev/null 2>&1 && break
    if ! kill -0 $WEAVE_PID >/dev/null 2>&1 ; then
        echo Weave process has died >&2
        exit 1
    fi
    sleep 1
done

reclaim_ips() {
    ID=$1
    shift
    for CIDR in "$@" ; do
        curl -s -S -X PUT "$HTTP_ADDR/ip/$ID/$CIDR" || true
    done
}

# Tell the newly-started weave about existing weave bridge IPs
/usr/bin/weaveutil container-addrs weave weave:expose | while read ID IFACE MAC IPS; do
    reclaim_ips "weave:expose" $IPS
done
# Tell weave about existing weave process IPs
/usr/bin/weaveutil process-addrs weave | while read ID IFACE MAC IPS; do
    reclaim_ips "_" $IPS
done

# Expose the weave network so host processes can communicate with pods
/home/weave/weave --local expose $WEAVE_EXPOSE_IP

wait $WEAVE_PID
