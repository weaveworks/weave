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
        run_on $host "sudo kubeadm reset && sudo rm -r -f /opt/cni/bin/*weave*"
    done 
}

function setup_kubernetes_cluster {
    teardown_kubernetes_cluster;

    greyly echo "Setting up kubernetes cluster"

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
        "$(dirname "$0")/../prog/weave-kube/weave-daemonset-k8s-1.7.yaml" | run_on "$HOST1" "$KUBECTL apply -n kube-system -f -"
}

function force_drop_node {
    greyly echo "Resetting node $1 with 'sudo kubeadm reset'" 
    run_on $1 "sudo kubeadm reset"
}

function weave_connected {
    run_on $HOST1 "curl -sS http://127.0.0.1:6784/status | grep \"$SUCCESS\""
}

#
# Test functions
#

function check_no_lost_ip_addresses {
    host=$1
    unreachable_count=$(run_on $host "sudo weave status ipam" | grep "unreachable" | wc -l)
    if [ "$unreachable_count" != "0" ]; then
        return 1 # fail
    fi
}

function main {
    WEAVE_IPAM_RECOVERY_DELAY_SECONDS=30

    start_suite "Test weave-net deallocates from IPAM on node failure";

    # ensure that we stop testing on first failure, but don't exit 
    # the script since we still have cleaning up to do
    set +e; ( set -e;

        setup_kubernetes_cluster;

        # Need to wait until all pods have come up
        assert_raises "wait_for_x weave_connected 'pods to be running weave net' $POD_SETUP_TIMEOUT_SECONDS"

        check_no_lost_ip_addresses $HOST1;
        check_no_lost_ip_addresses $HOST2;
        check_no_lost_ip_addresses $HOST3;

       
        force_drop_node $HOST2;

        sleep $WEAVE_IPAM_RECOVERY_DELAY_SECONDS;
        
        check_no_lost_ip_addresses $HOST1;
        check_no_lost_ip_addresses $HOST3;

    # Save exit status of subshell and resume terminating the script on a bad exit status
    ); status=$?; set -e
    
    teardown_kubernetes_cluster;

    end_suite;

    # Exit script with caught failure
    return $status;
}

main
