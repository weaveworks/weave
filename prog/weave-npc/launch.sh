#!/bin/sh

set -e

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
    if [ $mode = "nft" ]; then
      rm /sbin/iptables
      rm /sbin/iptables-save
      rm /sbin/iptables-restore
      ln /sbin/iptables-nft /sbin/iptables
      ln /sbin/iptables-nft-save /sbin/iptables-save
      ln /sbin/iptables-nft-restore /sbin/iptables-restore
    fi
}

setup_iptables_backend

# Start weave-npc with any flags specified in $EXTRA_ARGS as well as any flags passed to this container (for backwards compatibility)
exec /usr/bin/weave-npc $EXTRA_ARGS $@
