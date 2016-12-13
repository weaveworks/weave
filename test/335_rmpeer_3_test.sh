#! /bin/bash

. "$(dirname "$0")/config.sh"

start_suite "rmpeer reclaims IP addresses of lost peers"

# launch three peers, with all space belonging to the first
weave_on $HOST1 launch
PEER1=$(weave_on $HOST1 report -f '{{.Router.Name}}')
start_container $HOST1
weave_on $HOST2 launch $HOST1
weave_on $HOST2 prime
weave_on $HOST3 launch $HOST1
weave_on $HOST3 prime

# nuke 1st peer
docker_on $HOST1 kill weave

# transfer its space to the 2nd
weave_on $HOST2 rmpeer $PEER1
assert_raises "timeout 5 cat <( start_container $HOST2 )"

# ensure this is communicated promptly to the 3rd
assert_raises "timeout 5 cat <( start_container $HOST3 )"

end_suite
