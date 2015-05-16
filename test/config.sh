# NB only to be sourced

set -e

# Protect against being sourced multiple times to prevent
# overwriting assert.sh global state
if ! [ -z "$SOURCED_CONFIG_SH" ]; then
    return
fi
SOURCED_CONFIG_SH=true

# these ought to match what is in Vagrantfile
N_MACHINES=${N_MACHINES:-2}
IP_PREFIX=${IP_PREFIX:-192.168.48}
IP_SUFFIX_BASE=${IP_SUFFIX_BASE:-10}

if [ -z "$HOSTS" ] ; then
    for i in $(seq 1 $N_MACHINES); do
        IP="${IP_PREFIX}.$((${IP_SUFFIX_BASE}+$i))"
        HOSTS="$HOSTS $IP"
    done
fi

# these are used by the tests
HOST1=$(echo $HOSTS | cut -f 1 -d ' ')
HOST2=$(echo $HOSTS | cut -f 2 -d ' ')

. ./assert.sh

SSH=${SSH:-ssh -l vagrant -i ./insecure_private_key -o UserKnownHostsFile=./.ssh_known_hosts -o CheckHostIP=no -o StrictHostKeyChecking=no}

PING="ping -nq -W 1 -c 1"

remote() {
    rem=$1
    shift 1
    $@ > >(while read line; do echo -e "\e[0;34m$rem>\e[0m $line"; done)
}

colourise() {
    [ -t 0 ] && echo -ne '\e['$1'm' || true
    shift
    # It's important that we don't do this in a subshell, as some
    # commands we execute need to modify global state
    "$@"
    [ -t 0 ] && echo -ne '\e[0m' || true
}

whitely() {
    colourise '1;37' "$@"
}

greyly () {
    colourise '0;37' "$@"
}

redly() {
    colourise '1;31' "$@"
}

greenly() {
    colourise '1;32' "$@"
}

run_on() {
    host=$1
    shift 1
    greyly echo "Running on $host: $@" >&2
    remote $host $SSH $host $@
}

docker_on() {
    host=$1
    shift 1
    greyly echo "Docker on $host: $@" >&2
    docker -H tcp://$host:2375 $@
}

weave_on() {
    host=$1
    shift 1
    greyly echo "Weave on $host: $@" >&2
    DOCKER_HOST=tcp://$host:2375 $WEAVE $@
}

exec_on() {
    host=$1
    container=$2
    shift 2
    docker -H tcp://$host:2375 exec $container $@
}

start_container() {
    host=$1
    shift 1
    weave_on $host run $@ -t gliderlabs/alpine /bin/sh
}

start_container_with_dns() {
    host=$1
    shift 1
    weave_on $host run --with-dns $@ -t aanand/docker-dnsutils /bin/sh
}

container_ip() {
    weave_on $1 ps $2 | grep -o -E '[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}'
}

# assert_dns_record <host> <container> <name> <ip>
assert_dns_record() {
    assert "exec_on $1 $2 getent hosts $3 | tr -s ' '" "$4 $3"
    assert "exec_on $1 $2 getent hosts $4 | tr -s ' '" "$4 $3"
}

start_suite() {
    for host in $HOST1 $HOST2; do
        echo "Cleaning up on $host: removing all containers and resetting weave"
        containers=$(docker_on $host ps -aq 2>/dev/null)
        [ -n "$containers" ] && docker_on $host rm -f $containers >/dev/null 2>&1
        weave_on $host reset 2>/dev/null
    done
    whitely echo $@
}

end_suite() {
    whitely assert_end
}

WEAVE=../weave
DOCKER_NS=./bin/docker-ns
