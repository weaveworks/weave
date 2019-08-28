#!/bin/sh

set -e

# Start weave-npc with any flags specified in $EXTRA_ARGS as well as any flags passed to this container (for backwards compatibility)
exec /usr/bin/weave-npc $EXTRA_ARGS $@
