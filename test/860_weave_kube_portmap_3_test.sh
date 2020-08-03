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

function weave_connected {
    run_on $HOST1 "curl -sS http://127.0.0.1:6784/status | grep \"$SUCCESS\""
}

# setup a daemonset that uses hostport 80
function setup_hostport_daemonSet {
    greyly echo "Deploying daemonset that use hostport 80"
    run_on $HOST1 "$KUBECTL create -f -" <<EOF
apiVersion: extensions/v1beta1
kind: DaemonSet
metadata:
  name: frontend
spec:
  template:
    metadata:
      labels:
        app: frontend-webserver
    spec:
      containers:
        - name: webserver
          image: nginx
          ports:
          - containerPort: 80
            hostPort: 80
EOF
}

function check_daemonset_ready {
    ready=$($SSH $HOST1 "$KUBECTL get ds frontend  -o go-template='{{.status.numberReady}}'")
    if [ $ready = "2" ] ; then
        return 0
    fi
    return 1
}

#
# Suite
#
function main {
    start_suite "Test weave-net support for Kubernetes HostPort";

    setup_kubernetes_cluster;

    # Need to wait until all pods have come up
    assert_raises "wait_for_x weave_connected 'pods to be running weave net'"

    setup_hostport_daemonSet

    assert_raises 'wait_for_x check_daemonset_ready "wait for pods to be ready" 200'

    # verify hostport is accessible on each node
    assert_raises "run_on $HOST2 curl 127.0.0.1:80"
    assert_raises "run_on $HOST3 curl 127.0.0.1:80"

    teardown_kubernetes_cluster;

    end_suite;
}

main
