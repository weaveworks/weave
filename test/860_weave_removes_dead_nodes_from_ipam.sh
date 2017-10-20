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

function can_pods_communicate {
    podName=$($SSH $HOST1 "$KUBECTL get pods -l run=nettest -o go-template='{{(index .items 0).metadata.name}}'")
    status=$($SSH $HOST1 "$KUBECTL exec $podName -- curl -s -S http://127.0.0.1:8080/status")
    test "$status" = "pass" && return 0 || return 1
}


#
# Test functions
#

function check_all_pods_communicate {
    assert_raises 'wait_for_x can_pods_communicate pods'

    # Check that a pod can contact the outside world
    assert_raises "$SSH $HOST1 $KUBECTL exec $podName -- $PING 8.8.8.8"

    # Check that our pods haven't crashed
    assert "$SSH $HOST1 $KUBECTL get pods -n kube-system -l name=weave-net | grep -c Running" 3
}

function setup_pod_networking {
    # Set up a simple network policy so all our test pods can talk to each other
    run_on $HOST1 "$KUBECTL annotate ns default net.beta.kubernetes.io/network-policy='{\"ingress\":{\"isolation\":\"DefaultDeny\"}}'"
    run_on $HOST1 "$KUBECTL apply -f -" <<EOF
apiVersion: extensions/v1beta1
kind: NetworkPolicy
metadata:
  name: test840
spec:
  ingress:
  - from:
    - podSelector:
        matchExpressions:
        - key: run
          operator: In
          values:
          - nettest
    ports:
    - port: 8080
      protocol: TCP
  podSelector:
    matchLabels:
      run: nettest
EOF

    assert_raises 'wait_for_x check_ready "hosts to be ready"'

    # See if we can get some pods running that connect to the network
    run_on $HOST1 "$KUBECTL run --image-pull-policy=Never nettest --image=$IMAGE --replicas=3 -- -peers=3 -dns-name=nettest.default.svc.cluster.local."

    # Create a headless service so they can be found in Kubernetes DNS
    run_on $HOST1 "$KUBECTL create -f -" <<EOF
apiVersion: v1
kind: Service
metadata:
  name: nettest
spec:
  clusterIP: None
  ports:
  - port: 80
    targetPort: 8080
    protocol: TCP
  selector:
    run: nettest
EOF
}

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

function cleanup {
teardown_kubernetes_cluster
}
trap cleanup EXIT

function main {
    start_suite "Test weave-net deallocates IPAM on node failure";
    
    setup_kubernetes_cluster;

    sleep 5;

    setup_pod_networking;

    check_all_pods_communicate;

    # force_drop_node;

    # check_ipam_deallocated;

    teardown_kubernetes_cluster;
    
    end_suite;
}

main
