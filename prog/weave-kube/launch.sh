#!/bin/sh

set -e

# Default if not supplied - same as weave net default
IPALLOC_RANGE=${IPALLOC_RANGE:-10.32.0.0/12}
HTTP_ADDR=${WEAVE_HTTP_ADDR:-127.0.0.1:6784}
STATUS_ADDR=${WEAVE_STATUS_ADDR:-0.0.0.0:6782}

# Check if the IP range overlaps anything existing on the host
/usr/bin/weaveutil netcheck $IPALLOC_RANGE weave

# Default for network policy
EXPECT_NPC=${EXPECT_NPC:-1}

# kube-proxy requires that bridged traffic passes through netfilter
if [ ! -f /proc/sys/net/bridge/bridge-nf-call-iptables ] ; then
    echo /proc/sys/net/bridge/bridge-nf-call-iptables not found >&2
    exit 1
fi

echo 1 > /proc/sys/net/bridge/bridge-nf-call-iptables

SOURCE_BINARY=/usr/bin/weaveutil
VERSION=$(/home/weave/weaver $EXTRA_ARGS --version | sed -e 's/weave router //')
PLUGIN="weave-plugin-$VERSION"

install_cni_plugin() {
    mkdir -p $1 || return 1
    if [ ! -f "$1/$PLUGIN" ]; then
        cp "$SOURCE_BINARY" "$1/$PLUGIN"
    fi
}

upgrade_cni_plugin_symlink() {
    # Remove potential temporary symlink from previous failed upgrade:
    rm -f $1/$2.tmp
    # Atomically create a symlink to the plugin:
    ln -s "$1/$PLUGIN" $1/$2.tmp && mv -f $1/$2.tmp $1/$2
}

upgrade_cni_plugin() {
    # Check if weave-net and weave-ipam are (legacy) copies of the plugin, and
    # if so remove these so symlinks can be used instead from now onwards.
    if [ -f $1/weave-net  -a ! -L $1/weave-net  ];  then rm $1/weave-net;   fi
    if [ -f $1/weave-ipam -a ! -L $1/weave-ipam ];  then rm $1/weave-ipam;  fi

    # Create two symlinks to the plugin, as it has a different 
    # behaviour depending on its name:
    if [ "$(readlink -f $1/weave-net)" != "$1/$PLUGIN" ]; then
        upgrade_cni_plugin_symlink $1 weave-net
    fi
    if [ "$(readlink -f $1/weave-ipam)" != "$1/$PLUGIN" ]; then
        upgrade_cni_plugin_symlink $1 weave-ipam
    fi
}

# Install CNI plugin binary to typical CNI bin location
# with fall-back to CNI directory used by kube-up on GCI OS.
if install_cni_plugin /opt/cni/bin ; then
    upgrade_cni_plugin /opt/cni/bin
elif install_cni_plugin /host_home/kubernetes/bin ; then
    upgrade_cni_plugin /host_home/kubernetes/bin
else
    echo "Failed to install the Weave CNI plugin" >&2
    exit 1
fi

# Need to create bridge before running weaver so we can use the peer address
# (because of https://github.com/weaveworks/weave/issues/2480)
WEAVE_NPC_OPTS="--expect-npc"
if [ "${EXPECT_NPC}" = "0" ]; then
    WEAVE_NPC_OPTS=""
fi

/home/weave/weave --local create-bridge --force $WEAVE_NPC_OPTS

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

/home/weave/weaver $EXTRA_ARGS --port=6783 $BRIDGE_OPTIONS \
     --http-addr=$HTTP_ADDR --status-addr=$STATUS_ADDR --docker-api='' --no-dns \
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

# Expose the weave network so host processes can communicate with pods
/home/weave/weave --local expose $WEAVE_EXPOSE_IP

wait $WEAVE_PID
