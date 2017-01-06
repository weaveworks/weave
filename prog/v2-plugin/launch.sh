#!/bin/sh

set -e

# Default if not supplied - same as weave net default
IPALLOC_RANGE=${IPALLOC_RANGE:-10.32.0.0/12}
HTTP_ADDR=${WEAVE_HTTP_ADDR:-127.0.0.1:6784}
STATUS_ADDR=${WEAVE_STATUS_ADDR:-0.0.0.0:6782}
HOST_ROOT=${HOST_ROOT:-/host}
LOG_FILE=/tmp/weaver.log

echo "Starting launch.sh" >>$LOG_FILE

# Check if the IP range overlaps anything existing on the host
/usr/bin/weaveutil netcheck $IPALLOC_RANGE weave

/home/weave/weave --local create-bridge --force >>$LOG_FILE 2>&1

# ?
NICKNAME_ARG=""

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
    # ideally we would know the peer count
    IPALLOC_INIT="consensus"
fi

/home/weave/weaver $EXTRA_ARGS --port=6783 $BRIDGE_OPTIONS \
    --http-addr=$HTTP_ADDR --status-addr=$STATUS_ADDR --docker-api='' --no-dns \
    --ipalloc-range=$IPALLOC_RANGE $NICKNAME_ARG \
    --ipalloc-init $IPALLOC_INIT \
    "$@" \
    >>$LOG_FILE 2>&1 &
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

/home/weave/plugin --meshsocket='' --docker-api='' >/tmp/plugin.log 2>&1 &

reclaim_ips() {
    ID=$1
    shift
    for CIDR in "$@"; do
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

echo "End of launch.sh" >>$LOG_FILE

wait $WEAVE_PID
