#! /bin/bash

. "$(dirname "$0")/config.sh"

function howmany { echo $#; }

#
# Test vars
#
TOKEN=112233.4455667788990000
HOST1IP=$($SSH $HOST1 "getent hosts $HOST1 | cut -f 1 -d ' '")
NUM_HOSTS=$(howmany $HOSTS)
SUCCESS="$(( $NUM_HOSTS * ($NUM_HOSTS-1) )) established"
KUBECTL="sudo kubectl --kubeconfig /etc/kubernetes/admin.conf"
KUBE_PORT=6443
IMAGE=weaveworks/network-tester:latest

if [ -n "$COVERAGE" ]; then
    COVERAGE_ARGS="env:\\n                - name: EXTRA_ARGS\\n                  value: \"-test.coverprofile=/home/weave/cover.prof --\""
else
    COVERAGE_ARGS="env:"
fi

#
# Utility functions
#
function teardown_kubernetes_cluster {
    greyly echo "Tearing down kubernetes cluster"
    for host in $HOSTS; do
        run_on $host "sudo kubeadm reset --force && sudo rm -r -f /opt/cni/bin/*weave*"
    done
}

function setup_kubernetes_cluster {
    teardown_kubernetes_cluster;

    greyly echo "Setting up kubernetes cluster"

    # kubeadm init upgrades to latest Kubernetes version by default, therefore we try to lock the version using the below option:
    k8s_version="$(run_on $HOST1 "kubelet --version" | grep -oP "(?<=Kubernetes )v[\d\.\-beta]+")"

    for host in $HOSTS; do
        if [ $host = $HOST1 ] ; then
        run_on $host "sudo systemctl start kubelet && sudo kubeadm init --ignore-preflight-errors=NumCPU --kubernetes-version=$k8s_version --token=$TOKEN"
        else
        run_on $host "sudo systemctl start kubelet && sudo kubeadm join --token=$TOKEN --discovery-token-unsafe-skip-ca-verification $HOST1IP:$KUBE_PORT"
        fi
    done

    # Ensure Kubernetes uses locally built container images and inject code coverage environment variable (or do nothing depending on $COVERAGE):
    sed -e "s%imagePullPolicy: Always%imagePullPolicy: Never%" \
        -e "s%env:%$COVERAGE_ARGS%" \
        "$(dirname "$0")/../prog/weave-kube/weave-daemonset-k8s-1.7.yaml" | run_on "$HOST1" "$KUBECTL apply -n kube-system -f -"
}

function force_drop_node {
    greyly echo "Shutting down Kubernetes on node $1"
    run_on $1 "sudo kubeadm reset --force"
    greyly echo "Dropping node $1 with 'sudo kubectl delete node'"
    local target=$(echo "$1" | awk -F"." '{print $1}')
    run_on $HOST1 "$KUBECTL delete node $target"
}

function weave_connected {
    run_on $HOST1 "curl -sS http://127.0.0.1:6784/status | grep \"$SUCCESS\""
}

function unreachable_ip_addresses_count {
    # Note Well: This will return 0 if weave is not running at all.
    local host=$1
    local count=$(get_command_output_on $host "sudo weave status ipam | grep unreachable | wc -l")
    echo $count
}

function relaunch_weave_pod {
    local target=$(echo "$1" | awk -F"." '{print $1}')

    # This is a pretty complex jq query. Is there a simpler way to
    # get the weave-net pod name back using kubectl templates?
    local QUERY="$KUBECTL get pods -n kube-system -o json | jq -r \".items | map(select(.spec.nodeName == \\\"$target\\\")) | map(select(.metadata.labels.name == \\\"weave-net\\\")) | map(.metadata.name) | .[]\""
    local podname=$(get_command_output_on $HOST1 $QUERY) # Run on master node
    run_on $HOST1 "$KUBECTL get pods -n kube-system \"$podname\" -o yaml | $KUBECTL replace --force -f -"
}

#
# Suite
#
function main {
    local IPAM_RECOVER_DELAY=10

    start_suite "Test weave-net deallocates from IPAM on node failure";

    setup_kubernetes_cluster;

    # Need to wait until all pods have come up
    assert_raises "wait_for_x weave_connected 'pods to be running weave net'"

    greyly echo "Checking unreachable IPs"
    assert "unreachable_ip_addresses_count $HOST1" "0";
    assert "unreachable_ip_addresses_count $HOST2" "0";
    assert "unreachable_ip_addresses_count $HOST3" "0";

    force_drop_node $HOST2;

    sleep $IPAM_RECOVER_DELAY;

    greyly echo "Checking unreachable IPs"
    assert "unreachable_ip_addresses_count $HOST1" "0";
    assert "unreachable_ip_addresses_count $HOST3" "0";

    teardown_kubernetes_cluster;

    end_suite;
}

main
