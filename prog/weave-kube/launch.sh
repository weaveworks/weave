#!/bin/sh

set -e

modprobe_safe() {
    modprobe $1 || echo "Ignore the error if \"$1\" is built-in in the kernel" >&2
}

# Check whether xt_set actually exists
xt_set_exists() {
    # Clean everything up in advance, in case there's leftovers
    iptables -F WEAVE-KUBE-TEST 2>/dev/null || true
    iptables -X WEAVE-KUBE-TEST 2>/dev/null || true
    ipset destroy weave-kube-test 2>/dev/null || true

    ipset create weave-kube-test hash:ip
    iptables -t filter -N WEAVE-KUBE-TEST
    if ! iptables -A WEAVE-KUBE-TEST -m set --match-set weave-kube-test src -j DROP; then
        NOT_EXIST=1
    fi
    iptables -F WEAVE-KUBE-TEST
    iptables -X WEAVE-KUBE-TEST
    ipset destroy weave-kube-test
    [ -z "$NOT_EXIST" ] || (echo "\"xt_set\" does not exist" >&2 && return 1)
}

# Default if not supplied - same as weave net default
IPALLOC_RANGE=${IPALLOC_RANGE:-10.32.0.0/12}
HTTP_ADDR=${WEAVE_HTTP_ADDR:-127.0.0.1:6784}
STATUS_ADDR=${WEAVE_STATUS_ADDR:-0.0.0.0:6782}
HOST_ROOT=${HOST_ROOT:-/host}
CONN_LIMIT=${CONN_LIMIT:-30}
DB_PREFIX=${DB_PREFIX:-/weavedb/weave-net}

# Check if the IP range overlaps anything existing on the host
/usr/bin/weaveutil netcheck $IPALLOC_RANGE weave

# Default for network policy
EXPECT_NPC=${EXPECT_NPC:-1}

# Ensure we have the required modules for NPC
if [ "${EXPECT_NPC}" != "0" ]; then
    modprobe_safe br_netfilter
    modprobe_safe xt_set
    xt_set_exists
fi

# kube-proxy requires that bridged traffic passes through netfilter
if ! BRIDGE_NF_ENABLED=$(cat /proc/sys/net/bridge/bridge-nf-call-iptables); then
    echo "Cannot detect bridge-nf support - network policy and iptables mode kubeproxy may not work reliably" >&2
else
    if [ "$BRIDGE_NF_ENABLED" != "1" ]; then
        echo 1 > /proc/sys/net/bridge/bridge-nf-call-iptables
    fi
fi

# Default is that npc will be running; allow caller to override
WEAVE_NPC_OPTS="--expect-npc"
if [ "${EXPECT_NPC}" = "0" ]; then
    WEAVE_NPC_OPTS=""
fi

# Kubernetes sets HOSTNAME to the host's hostname
# when running a pod in host namespace.
NICKNAME_ARG=""
if [ -n "$HOSTNAME" ] ; then
    NICKNAME_ARG="--nickname=$HOSTNAME"
fi

# Router password. You can create a Kubernetes secret and specify that as the
# weave password through the WEAVE_SECRET environment variable.
PASSWORD_ARG=""
if [ -n "$WEAVE_SECRET" ] ; then
    PASSWORD_ARG="--password=$WEAVE_SECRET"
fi

router_bridge_opts() {
    echo --datapath=datapath
    [ -z "$WEAVE_MTU" ] || echo --mtu "$WEAVE_MTU"
    [ -z "$WEAVE_NO_FASTDP" ] || echo --no-fastdp
}

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

# Find out what peer name we will be using
PEERNAME=$(/home/weave/weaver $EXTRA_ARGS --print-peer-name --host-root=$HOST_ROOT --db-prefix="$DB_PREFIX")
if [ -z "$PEERNAME" ]; then
    echo "Unable to get peer name" >&2
    exit 2
fi

# If we have persisted data and peer reclaim is in use
if [ -f ${DB_PREFIX}data.db ]; then
    if /usr/bin/weaveutil get-db-flag "$DB_PREFIX" peer-reclaim >/dev/null ; then
        # If this peer name is not stored in the list then we were removed by another peer.
        if /home/weave/kube-peers -check-peer-new -peer-name="$PEERNAME" -log-level=debug ; then
            # In order to avoid a CRDT clash, clean up
            echo "Peer not in list; removing persisted data" >&2
            rm -f ${DB_PREFIX}data.db
        fi
    fi
fi

# flag that peer reclaim is now in use - note we have to do this before
# weaver is running as the DB can only be opened by one process at a time
/usr/bin/weaveutil set-db-flag "$DB_PREFIX" peer-reclaim ok

post_start_actions() {
    # Wait for weave process to become responsive
    while true ; do
        curl $HTTP_ADDR/status >/dev/null 2>&1 && break
        sleep 1
    done

    # Install CNI plugin binary to typical CNI bin location
    # with fall-back to CNI directory used by kube-up on GCI OS
    if ! mkdir -p $HOST_ROOT/opt/cni/bin ; then
        if mkdir -p $HOST_ROOT/home/kubernetes/bin ; then
            export WEAVE_CNI_PLUGIN_DIR=$HOST_ROOT/home/kubernetes/bin
        else
            echo "Failed to install the Weave CNI plugin" >&2
            exit 1
        fi
    fi
    mkdir -p $HOST_ROOT/etc/cni/net.d
    export HOST_ROOT
    /home/weave/weave --local setup-cni

    # Attempt to run the reclaim process, but don't halt the script if it fails
    /home/weave/kube-peers -reclaim -node-name="$HOSTNAME" -peer-name="$PEERNAME" -log-level=debug || true

    # Expose the weave network so host processes can communicate with pods
    /home/weave/weave --local expose $WEAVE_EXPOSE_IP
}

post_start_actions &

/home/weave/weaver $EXTRA_ARGS --port=6783 $(router_bridge_opts) \
     --name="$PEERNAME" \
     --host-root=$HOST_ROOT \
     --http-addr=$HTTP_ADDR --status-addr=$STATUS_ADDR --docker-api='' --no-dns \
     --db-prefix="$DB_PREFIX" \
     --ipalloc-range=$IPALLOC_RANGE $NICKNAME_ARG $PASSWORD_ARG \
     --ipalloc-init $IPALLOC_INIT \
     --conn-limit=$CONN_LIMIT \
     $WEAVE_NPC_OPTS \
     "$@" \
     $KUBE_PEERS
