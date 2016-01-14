# NB only to be sourced

set -e

. "../tools/integration/config.sh"

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

DNS_IMAGE="aanand/docker-dnsutils"
TEST_IMAGES="$TEST_IMAGES $DNS_IMAGE"

CHECK_ETHWE_UP="grep ^1$ /sys/class/net/ethwe/carrier"
CHECK_ETHWE_MISSING="test ! -d /sys/class/net/ethwe"

upload_executable() {
    host=$1
    file=$2
    target=${3:-/usr/local/bin/$(basename "$file")}
    dir=$(dirname "$target")
    run_on $host "[ -e '$dir' ] || sudo mkdir -p '$dir'"
    [ -z "$DEBUG" ] || greyly echo "Uploading to $host: $file -> $target" >&2
    <"$file" remote $host $SSH $host sh -c "cat | sudo tee $target >/dev/null"
    run_on $host "sudo chmod a+x $target"
}

docker_api_on() {
    host=$1
    method=$2
    url=$3
    data=$4
    shift 4
    [ -z "$DEBUG" ] || greyly echo "Docker (API) on $host:$DOCKER_PORT: $method $url" >&2
    echo -n "$data" | curl -s -f -X "$method" -H Content-Type:application/json "http://$host:$DOCKER_PORT/v1.15$url" -d @-
}

proxy() {
    DOCKER_PORT=12375 "$@"
}

stop_router_on() {
    host=$1
    shift 1
    # we don't invoke `weave stop-router` here because that removes
    # the weave container, which means we a) can't grab coverage
    # stats, and b) can't inspect the logs when tests fail.
    docker_on $host stop weave 1>/dev/null 2>&1 || true
    docker_on $host stop weaveproxy 1>/dev/null 2>&1 || true
    if [ -n "$COVERAGE" ] ; then
        collect_coverage $host weave
        collect_coverage $host weaveproxy
    fi
}

start_container() {
    host=$1
    shift 1
    weave_on $host run "$@" -t $SMALL_IMAGE /bin/sh
}

start_container_with_dns() {
    host=$1
    shift 1
    weave_on $host run "$@" -t $DNS_IMAGE /bin/sh
}

start_container_local_plugin() {
    host=$1
    shift 1
    # using ssh rather than docker -H because CircleCI docker client is older
    $SSH $host docker run "$@" -dt --net=weave $SMALL_IMAGE /bin/sh
}

proxy_start_container() {
    host=$1
    shift 1
    proxy docker_on $host run "$@" -dt $SMALL_IMAGE /bin/sh
}

proxy_start_container_with_dns() {
    host=$1
    shift 1
    proxy docker_on $host run "$@" -dt $DNS_IMAGE /bin/sh
}

container_ip() {
    weave_on $1 ps $2 | grep -o -E '[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}'
}

# assert_dns_record <host> <container> <name> [<ip> ...]
assert_dns_record() {
    local host=$1
    local container=$2
    local name=$3
    shift 3
    exp_ips_regex=$(echo "$@" | sed -e 's/ /\\\|/g')

    [ -z "$DEBUG" ] || greyly echo "Checking whether $name exists at $host:$container"
    assert_raises "exec_on $host $container getent hosts $name | grep -q '$exp_ips_regex'"

    [ -z "$DEBUG" ] || greyly echo "Checking whether the IPs '$@' exists at $host:$container"
    for ip in "$@" ; do
        assert "exec_on $host $container getent hosts $ip | tr -s ' ' | tr '[:upper:]' '[:lower:]'" "$(echo $ip $name | tr '[:upper:]' '[:lower:]')"
    done
}

# assert_no_dns_record <host> <container> <name>
assert_no_dns_record() {
    host=$1
    container=$2
    name=$3

    [ -z "$DEBUG" ] || greyly echo "Checking if '$name' does not exist at $host:$container"
    assert_raises "exec_on $host $container getent hosts $name" 2
}

# assert_dns_a_record <host> <container> <name> <ip> [<expected_name>]
assert_dns_a_record() {
    exp_name=${5:-$3}
    assert "exec_on $1 $2 getent hosts $3 | tr -s ' ' | cut -d ' ' -f 1,2" "$4 $exp_name"
}

# assert_dns_ptr_record <host> <container> <name> <ip>
assert_dns_ptr_record() {
    assert "exec_on $1 $2 getent hosts $4 | tr -s ' '" "$4 $3"
}

end_suite() {
    whitely assert_end
    for host in $HOSTS; do
        stop_router_on $host
    done
}

collect_coverage() {
    host=$1
    container=$2
    mkdir -p ./coverage
    rm -f cover.prof
    docker_on $host cp $container:/home/weave/cover.prof . 2>/dev/null || return 0
    # ideally we'd know the name of the test here, and put that in the filename
    mv cover.prof $(mktemp -u ./coverage/integration.XXXXXXXX) || true
}

WEAVE=$DIR/../weave
DOCKER_NS=$DIR/../bin/docker-ns
