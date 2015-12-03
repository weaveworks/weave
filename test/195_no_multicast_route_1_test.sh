#! /bin/bash

. ./config.sh

show_multicast_route_on() {
    exec_on $1 $2 ip route show 224.0.0.0/4
}

start_suite "--no-multicast-route operation"

# Ensure containers run either way have no multicast route
weave_on $HOST1 launch-router
weave_on $HOST1 launch-proxy --no-multicast-route

start_container $HOST1 --no-multicast-route --name c1
proxy_start_container $HOST1 --name c2

assert "show_multicast_route_on $HOST1 c1"
assert "show_multicast_route_on $HOST1 c2"

# Ensure current proxy options are obeyed on container start
docker_on $HOST1 stop c2
weave_on $HOST1 stop-proxy
weave_on $HOST1 launch-proxy

proxy docker_on $HOST1 start c2

assert "show_multicast_route_on $HOST1 c2" "224.0.0.0/4 dev ethwe "

end_suite
