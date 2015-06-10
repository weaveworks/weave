#! /bin/bash

. ./config.sh

# Docker inspect hostname + domainname of container $2 on host $1
docker_inspect_fqdn() {
    docker_on $1 inspect --format='{{.Config.Hostname}}.{{.Config.Domainname}}' $2
}

start_suite "Use container name as hostname"

weave_on $HOST1 launch -iprange 10.2.0.0/24

start_container_with_dns $HOST1 --name=c1-name
start_container_with_dns $HOST1 --name=c2-name -h c2-hostname.weave.local
start_container_with_dns $HOST1 --name=c3-name --hostname=c3-hostname.weave.local
start_container_with_dns $HOST1 -h c4-hostname.weave.local --name=c4-name
start_container_with_dns $HOST1 --hostname=c5-hostname.weave.local --name=c5-name

assert "docker_inspect_fqdn $HOST1 c1-name" "c1-name.weave.local"
assert "docker_inspect_fqdn $HOST1 c2-name" "c2-hostname.weave.local"
assert "docker_inspect_fqdn $HOST1 c3-name" "c3-hostname.weave.local"
assert "docker_inspect_fqdn $HOST1 c4-name" "c4-hostname.weave.local"
assert "docker_inspect_fqdn $HOST1 c5-name" "c5-hostname.weave.local"

end_suite
