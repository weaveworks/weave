
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
. "$DIR/config.sh"

IPERF_IMAGE="inercia/iperf"

################################
# performance utils
################################

start_perf_suite() {
    start_suite $@
    [ -d $DIR/performance ] || mkdir $DIR/performance
}

end_perf_suite() {
    for host in $HOSTS; do
        docker_on $host stop iperf_server 1>/dev/null 2>&1 || true
        docker_on $host stop iperf_client 1>/dev/null 2>&1 || true
    done
    end_suite $@
}

# start an iperf server
start_iperf_server() {
    host=$1
    addr=$2
    shift 2
    weave_on $host run $addr --name=iperf_server -t $IPERF_IMAGE "$@"
}

# start and wait for an iperf client
start_iperf_client() {
    host=$1
    addr=$2
    name=$3
    shift 3

    C=$(weave_on $host run $addr --name=iperf_client -t $IPERF_IMAGE -c iperf_server -y C -i1 "$@")
    docker_on $host wait $C 1>/dev/null 2>&1

    # collect the stats and leave them in the performance/ dir
    rm -f $DIR/performance/$name
    docker_on $host logs $C > $DIR/performance/$name.csv
    echo $C
}

