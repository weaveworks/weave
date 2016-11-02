#! /bin/bash

. ./config.sh

tear_down_kubeadm() {
    for host in $HOSTS; do
        # If we don't stop kubelet, it will restart all the containers we're trying to kill
        run_on $host "sudo systemctl stop kubelet"
        rm_containers $host $(docker_on $host ps -aq)
        run_on $host "test ! -d /var/lib/kubelet || sudo find /var/lib/kubelet -execdir findmnt -n -t tmpfs -o TARGET -T {} \; | uniq | xargs -r sudo umount"
        run_on $host "sudo rm -r -f /etc/kubernetes /var/lib/kubelet /var/lib/etcd"
    done
}

start_suite "Test weave-kube image with Kubernetes"

TOKEN=112233.445566778899000
HOST1IP=$($SSH $HOST1 "getent hosts $HOST1 | cut -f 1 -d ' '")
SUCCESS="6 established"

tear_down_kubeadm

run_on $HOST1 "sudo systemctl start kubelet && sudo kubeadm init --token=$TOKEN"
run_on $HOST2 "sudo systemctl start kubelet && sudo kubeadm join --token=$TOKEN $HOST1IP"
run_on $HOST3 "sudo systemctl start kubelet && sudo kubeadm join --token=$TOKEN $HOST1IP"

sed -e 's/imagePullPolicy: Always/imagePullPolicy: Never/' ../prog/weave-kube/weave-daemonset.yaml | run_on $HOST1 "kubectl apply -f -"

sleep 5

wait_for_connections() {
    for i in $(seq 1 30); do
        if run_on $HOST1 "curl -sS http://127.0.0.1:6784/status | grep \"$SUCCESS\"" ; then
            return
        fi
        echo "Waiting for connections"
        sleep 1
    done
    echo "Timed out waiting for connections to establish" >&2
    exit 1
}

assert_raises wait_for_connections

# See if we can get some pods running that connect to the network
run_on $HOST1 "kubectl run hello --image=weaveworks/hello-world --replicas=3"

wait_for_pods() {
    for i in $(seq 1 30); do
        if run_on $HOST1 "kubectl get pods | grep 'hello.*Running'" ; then
            return
        fi
        echo "Waiting for pods"
        sleep 1
    done
    echo "Timed out waiting for pods" >&2
    exit 1
}

assert_raises wait_for_pods

tear_down_kubeadm

end_suite
