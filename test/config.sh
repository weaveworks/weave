# NB only to be sourced

set -e

. ./assert.sh

SSH=${SSH:-ssh -l vagrant -i ./insecure_private_key}
HOST1=${HOST1:-192.168.48.19}
HOST2=${HOST2:-192.168.48.15}

remote() {
    rem=$1
    shift 1
    $@ > >(while read line; do echo -e "\e[0;34m$rem>\e[0m $line"; done)
}

whitely() {
    echo -e '\e[1;37m'`$@`'\e[0m'
}

greyly () {
    echo -e '\e[0;37m'`$@`'\e[0m'
}

run_on() {
    host=$1
    shift 1
    greyly echo "Running on $host: $@"
    remote $host $SSH $host $@
}

docker_on() {
    host=$1
    shift 1
    greyly echo "Docker on $host: $@"
    docker -H tcp://$host:2375 $@
}

start_suite() {
    whitely echo $@
}

end_suite() {
    whitely assert_end
}

WEAVE=./bin/weave
