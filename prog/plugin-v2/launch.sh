#!/bin/sh

set -e

# Default if not supplied - same as weave net default
IPALLOC_RANGE=${IPALLOC_RANGE:-10.32.0.0/12}
HTTP_ADDR=${WEAVE_HTTP_ADDR:-127.0.0.1:6784}
STATUS_ADDR=${WEAVE_STATUS_ADDR:-0.0.0.0:6782}
HOST_ROOT=${HOST_ROOT:-/host}
WEAVE_DIR="/host/var/lib/weave"
LOG_FILE_WEAVER="$WEAVE_DIR/weaver.log"
LOG_FILE_PLUGIN="$WEAVE_DIR/plugin.log"

mkdir $WEAVE_DIR || true

echo "Starting launch.sh" >>$LOG_FILE_WEAVER

# Check if the IP range overlaps anything existing on the host
/usr/bin/weaveutil netcheck $IPALLOC_RANGE weave

SWARM_PEERS=$(/usr/bin/weaveutil swarm-manager-peers 2>>$LOG_FILE_WEAVER)
! /usr/bin/weaveutil is-swarm-manager 2>>$LOG_FILE_WEAVER || IS_SWARM_MANAGER=1
# Prevent from restoring from a persisted peers list
rm -f "/restart.sentinel"

/home/weave/weave --local create-bridge \
                          --proc-path=/host/proc \
                          --weavedb-dir-path=$WEAVE_DIR \
                          --force >>$LOG_FILE_WEAVER 2>&1

BRIDGE_OPTIONS="--datapath=datapath"
if [ "$(/home/weave/weave --local bridge-type)" = "bridge" ]; then
    # TODO: Call into weave script to do this
    if ! ip link show vethwe-pcap >/dev/null 2>&1; then
        ip link add name vethwe-bridge type veth peer name vethwe-pcap
        ip link set vethwe-bridge up
        ip link set vethwe-pcap up
        ip link set vethwe-bridge master weave
    fi
    BRIDGE_OPTIONS="--iface=vethwe-pcap"
fi

if [ -z "$IPALLOC_INIT" ]; then
    if [ $IS_SWARM_MANAGER == "1" ]; then
        IPALLOC_INIT="consensus=$(echo $SWARM_PEERS | wc -l)"
    else
        IPALLOC_INIT="observer"
    fi
fi

/home/weave/weaver $EXTRA_ARGS --port=6783 $BRIDGE_OPTIONS \
    --http-addr=$HTTP_ADDR --status-addr=$STATUS_ADDR --docker-api='' --no-dns \
    --ipalloc-range=$IPALLOC_RANGE \
    --ipalloc-init $IPALLOC_INIT \
    --nickname "$(hostname)" \
    --log-level=debug \
    --db-prefix="$WEAVE_DIR/weave" \
    $(echo $SWARM_PEERS | tr '\n' ' ') \
    >>$LOG_FILE_WEAVER 2>&1 &
WEAVE_PID=$!

# Wait for weave process to become responsive
while true; do
    curl $HTTP_ADDR/status >/dev/null 2>&1 && break
    if ! kill -0 $WEAVE_PID >/dev/null 2>&1; then
        echo Weave process has died >&2
        exit 1
    fi
    sleep 1
done

# TODO(mp) docker api?
/home/weave/plugin $EXTRA_ARGS --log-level=debug --meshsocket='' --docker-api='' >>$LOG_FILE_PLUGIN 2>&1 &

echo "End of launch.sh" >>$LOG_FILE_WEAVER

wait $WEAVE_PID
