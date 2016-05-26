#! /bin/bash

. ./config.sh

start_suite "Check resume uses persisted discovered peers"

# Create a 'chain' of direct connections, letting the full mesh establish via
# discovery. We set the consensus value in such a way that all three peers are
# needed - this way `weave prime` blocks until the mesh is fully established.
weave_on $HOST1 launch --ipalloc-init consensus=4
weave_on $HOST2 launch --ipalloc-init consensus=4 $HOST1
weave_on $HOST3 launch --ipalloc-init consensus=4 $HOST2
weave_on $HOST1 prime

# Stop them all
weave_on $HOST1 stop
weave_on $HOST2 stop
weave_on $HOST3 stop

# Resume first and last nodes in the chain
weave_on $HOST1 launch --resume
weave_on $HOST3 launch --resume

# Ensure they're connected
start_container $HOST1 --name c1
start_container $HOST3 --name c3

assert_raises "exec_on $HOST1 c1 $PING $(container_ip $HOST3 c3)"

end_suite
