#!/bin/sh
# Initialisation of Weave Net pod: check Linux settings and install CNI plugin

set -e

[ -n "$WEAVE_DEBUG" ] && set -x

modprobe_safe() {
    modprobe $1 || echo "Ignore the error if \"$1\" is built-in in the kernel" >&2
}

# Check whether xt_set actually exists
xt_set_exists() {
    # Clean everything up in advance, in case there's leftovers
    iptables -w -F WEAVE-KUBE-TEST 2>/dev/null || true
    iptables -w -X WEAVE-KUBE-TEST 2>/dev/null || true
    ipset destroy weave-kube-test 2>/dev/null || true

    ipset create weave-kube-test hash:ip
    iptables -w -t filter -N WEAVE-KUBE-TEST
    if ! iptables -w -A WEAVE-KUBE-TEST -m set --match-set weave-kube-test src -j DROP; then
        NOT_EXIST=1
    fi
    iptables -w -F WEAVE-KUBE-TEST
    iptables -w -X WEAVE-KUBE-TEST
    # delay to allow kernel to clean up - see https://github.com/weaveworks/weave/issues/3847
    sleep 1
    ipset destroy weave-kube-test
    [ -z "$NOT_EXIST" ] || (echo "\"xt_set\" does not exist" >&2 && return 1)
}

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

# This is where we expect the manifest to map host directories
HOST_ROOT=${HOST_ROOT:-/host}

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