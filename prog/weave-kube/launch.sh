#!/bin/sh
# Launch of Weave Net pod - requires that init.sh has been run previously

set -e

[ -n "$WEAVE_DEBUG" ] && set -x

# If this is run from an older manifest, run the init script here
[ "${INIT_CONTAINER}" = "true" ] || "$(dirname "$0")/init.sh"

# Setup iptables backend to be legacy or nftable
setup_iptables_backend() {
    if [ -n "${IPTABLES_BACKEND}" ]; then
      mode=$IPTABLES_BACKEND
    else
      # auto-detect if iptables backend mode to use if not specified explicitly
      num_legacy_lines=$( (iptables-legacy-save || true) 2>/dev/null | grep '^-' | wc -l)
      num_nft_lines=$( (iptables-nft-save || true) 2>/dev/null | grep '^-' | wc -l)
      if [ "${num_legacy_lines}" -ge 10 ]; then
        mode="legacy"
      else
        if [ "${num_legacy_lines}" -ge "${num_nft_lines}" ]; then
          mode="legacy"
        else
          mode="nft"
        fi
      fi
    fi
    printf "iptables backend mode: %s\n" "$mode"
    # The weave-kube image uses the iptables-nft tools as default
    # from weave 2.9.0 onwards, but includes the legacy tools. If
    # legacy is detected, change the default symlinks.
    if [ "$mode" = "legacy" ]; then
      [ -n "$WEAVE_DEBUG" ] && echo "Changing iptables symlinks..."
      rm /sbin/iptables
      rm /sbin/iptables-save
      rm /sbin/iptables-restore
      ln -s /sbin/iptables-legacy /sbin/iptables
      ln -s /sbin/iptables-legacy-save /sbin/iptables-save
      ln -s /sbin/iptables-legacy-restore /sbin/iptables-restore
    fi
}

setup_iptables_backend

# Default if not supplied - same as weave net default
IPALLOC_RANGE=${IPALLOC_RANGE:-10.32.0.0/12}
HTTP_ADDR=${WEAVE_HTTP_ADDR:-127.0.0.1:6784}
METRICS_ADDR=${WEAVE_METRICS_ADDR:-0.0.0.0:6782}
HOST_ROOT=${HOST_ROOT:-/host}
CONN_LIMIT=${CONN_LIMIT:-200}
DB_PREFIX=${DB_PREFIX:-/weavedb/weave-net}

# Check if the IP range overlaps anything existing on the host
/usr/bin/weaveutil netcheck $IPALLOC_RANGE weave

# Default for network policy
EXPECT_NPC=${EXPECT_NPC:-1}

STATUS_OPTS="--metrics-addr=$METRICS_ADDR"
# --status-addr exposes internal information, so only turn it on if asked to.
if [ -n "${WEAVE_STATUS_ADDR}" ]; then
    STATUS_OPTS="$STATUS_OPTS --status-addr=$WEAVE_STATUS_ADDR"
fi

# Default is that npc will be running; allow caller to override
WEAVE_NPC_OPTS="--expect-npc"
if [ "${EXPECT_NPC}" = "0" ]; then
    WEAVE_NPC_OPTS=""
fi

NO_MASQ_LOCAL_OPT="--no-masq-local"
if [ "${NO_MASQ_LOCAL}" = "0" ]; then
    NO_MASQ_LOCAL_OPT=""
fi

# Kubernetes sets HOSTNAME to the host's hostname
# when running a pod in host namespace.
NICKNAME_ARG=""
if [ -n "$HOSTNAME" ] ; then
    NICKNAME_ARG="--nickname=$HOSTNAME"
fi

router_bridge_opts() {
    echo --datapath=datapath
    [ -z "$WEAVE_MTU" ] || echo --mtu "$WEAVE_MTU"
    [ -z "$WEAVE_NO_FASTDP" ] || echo --no-fastdp
}

if [ -z "$KUBE_PEERS" ]; then
    if ! KUBE_PEERS=$(/home/weave/kube-utils); then
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
        if /home/weave/kube-utils -check-peer-new -peer-name="$PEERNAME" -log-level=debug ; then
            # In order to avoid a CRDT clash, clean up
            echo "Peer not in list; removing persisted data" >&2
            rm -f ${DB_PREFIX}data.db
        fi
    fi
fi

# flag that peer reclaim is now in use - note we have to do this before
# weaver is running as the DB can only be opened by one process at a time
/usr/bin/weaveutil set-db-flag "$DB_PREFIX" peer-reclaim ok

# couple more variables
WEAVE_KUBERNETES_VERSION=$(/home/weave/kube-utils -print-k8s-version)
WEAVE_KUBERNETES_UID=$(/home/weave/kube-utils -print-uid)

post_start_actions() {
    # Wait for weave process to become responsive
    while true ; do
        curl $HTTP_ADDR/status >/dev/null 2>&1 && break
        sleep 1
    done

    # Attempt to run the reclaim process, but don't halt the script if it fails
    /home/weave/kube-utils -reclaim -node-name="$HOSTNAME" -peer-name="$PEERNAME" -log-level=debug || true

    # Expose the weave network so host processes can communicate with pods
    /home/weave/weave --local expose $WEAVE_EXPOSE_IP

    # Mark network as up
    /home/weave/kube-utils -set-node-status -node-name="$HOSTNAME"

    /home/weave/kube-utils -run-reclaim-daemon -node-name="$HOSTNAME" -peer-name="$PEERNAME" -log-level=debug&
}

post_start_actions &

/home/weave/weaver $EXTRA_ARGS --port=6783 $(router_bridge_opts) \
     --name="$PEERNAME" \
     --http-addr=$HTTP_ADDR $STATUS_OPTS --docker-api='' --no-dns \
     --db-prefix="$DB_PREFIX" \
     --ipalloc-range=$IPALLOC_RANGE $NICKNAME_ARG \
     --ipalloc-init $IPALLOC_INIT \
     --conn-limit=$CONN_LIMIT \
     $WEAVE_NPC_OPTS \
     $NO_MASQ_LOCAL_OPT \
     "$@" \
     $KUBE_PEERS
