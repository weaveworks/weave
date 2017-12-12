#!/bin/sh

set -e

# Default if not supplied - same as weave net default
IPALLOC_RANGE=${IPALLOC_RANGE:-10.32.0.0/12}
HTTP_ADDR=${WEAVE_HTTP_ADDR:-127.0.0.1:6784}
STATUS_ADDR=${WEAVE_STATUS_ADDR:-0.0.0.0:6782}
HOST_ROOT=${HOST_ROOT:-/host}
LOG_LEVEL=${LOG_LEVEL:-info}
WEAVE_DIR="/host/var/lib/weave"

mkdir $WEAVE_DIR || true

echo "Starting launch.sh"

# Check if the IP range overlaps anything existing on the host
/usr/bin/weaveutil netcheck $IPALLOC_RANGE weave

# We need to get a list of Swarm nodes which might run the net-plugin:
# - In the case of missing restart.sentinel, we assume that net-plugin has started
#   for the first time via the docker-plugin cmd. So it's safe to request docker.sock.
# - If restart.sentinel present, let weaver restore from it as docker.sock is not
#   available to any plugin in general (https://github.com/moby/moby/issues/32815).

PEERS=
if [ ! -f "/restart.sentinel" ]; then
    PEERS=$(/usr/bin/weaveutil swarm-manager-peers)
fi

router_bridge_opts() {
    echo --datapath=datapath
    [ -z "$WEAVE_MTU" ] || echo --mtu "$WEAVE_MTU"
    [ -z "$WEAVE_NO_FASTDP" ] || echo --no-fastdp
}

multicast_opt() {
    [ -z "$WEAVE_MULTICAST" ] || echo "--plugin-v2-multicast"
}

exec /home/weave/weaver $EXTRA_ARGS --port=6783 $(router_bridge_opts) \
    --host-root=/host \
    --proc-path=/host/proc \
    --http-addr=$HTTP_ADDR --status-addr=$STATUS_ADDR \
    --no-dns \
    --ipalloc-range=$IPALLOC_RANGE \
    --nickname "$(hostname)" \
    --log-level=$LOG_LEVEL \
    --db-prefix="$WEAVE_DIR/weave" \
    --plugin-v2 \
    $(multicast_opt) \
    --plugin-mesh-socket='' \
    --docker-api='' \
    $PEERS
