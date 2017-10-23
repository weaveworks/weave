#! /bin/bash

. "$(dirname "$0")/config.sh"

howmany() { echo $#; }

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
tear_down_kubeadm() {
    for host in $HOSTS; do
        run_on $host "sudo kubeadm reset && sudo rm -r -f /opt/cni/bin/*weave*"
    done
}



check_connections() {
    run_on $HOST1 "curl -sS http://127.0.0.1:6784/status | grep \"$SUCCESS\""
}

check_ready() {
    run_on $HOST1 "$KUBECTL get nodes | grep -c -w Ready | grep $NUM_HOSTS"
}

#
# Test functions
#

function setup_kubernetes_cluster {
    greyly echo "Setting up kubernetes cluster"
    tear_down_kubeadm;

    # Make an ipset, so we can check it doesn't get wiped out by Weave Net
    docker_on $HOST1 run --rm --privileged --net=host --entrypoint=/usr/sbin/ipset weaveworks/weave-npc create test_840_ipset bitmap:ip range 192.168.1.0/24 || true
    docker_on $HOST1 run --rm --privileged --net=host --entrypoint=/usr/sbin/ipset weaveworks/weave-npc add test_840_ipset 192.168.1.11

    # kubeadm init upgrades to latest Kubernetes version by default, therefore we try to lock the version using the below option:
    k8s_version="$(run_on $HOST1 "kubelet --version" | grep -oP "(?<=Kubernetes )v[\d\.\-beta]+")"
    k8s_version_option="$([[ "$k8s_version" > "v1.6" ]] && echo "kubernetes-version" || echo "use-kubernetes-version")"

    for host in $HOSTS; do
        if [ $host = $HOST1 ] ; then
        run_on $host "sudo systemctl start kubelet && sudo kubeadm init --$k8s_version_option=$k8s_version --token=$TOKEN"
        else
        run_on $host "sudo systemctl start kubelet && sudo kubeadm join --token=$TOKEN $HOST1IP:$KUBE_PORT"
        fi
    done

    # Ensure Kubernetes uses locally built container images and inject code coverage environment variable (or do nothing depending on $COVERAGE):
    sed -e "s%imagePullPolicy: Always%imagePullPolicy: Never%" \
        -e "s%env:%$COVERAGE_ARGS%" \
        "$(dirname "$0")/../prog/weave-kube/weave-daemonset-k8s-1.6.yaml" | run_on "$HOST1" "$KUBECTL apply -n kube-system -f -"
}

function teardown_kubernetes_cluster {
    greyly echo "Tearing down kubernetes cluster"
    tear_down_kubeadm; 

    # Destroy our test ipset, and implicitly check it is still there
    assert_raises "docker_on $HOST1 run --rm --privileged --net=host --entrypoint=/usr/sbin/ipset weaveworks/weave-npc destroy test_840_ipset"

}

function check_no_lost_ip_addresses {
    for host in $HOSTS; do
        unreachable_count=$(run_on $host "sudo weave status ipam" | grep "unreachable" | wc -l)
        if [ "$unreachable_count" -gt "0" ]; then
            return 1 # fail
        fi
    done
}

function force_drop_node {
    run_on $HOST2 "sudo kubeadm reset"
}

function cleanup {
teardown_kubernetes_cluster
}
trap cleanup EXIT

function main {
    WEAVE_IPAM_RECOVERY_DELAY=5

    start_suite "Test weave-net deallocates from IPAM on node failure";
    
    setup_kubernetes_cluster;

    sleep 5;

    check_no_lost_ip_addresses;

    force_drop_node;

    sleep ${WEAVE_IPAM_RECOVERY_DELAY};

    check_no_lost_ip_addresses;

    teardown_kubernetes_cluster;
    
    end_suite;
}

main
