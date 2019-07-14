#! /bin/bash

. "$(dirname "$0")/config.sh"

tear_down_kubeadm() {
    for host in $HOSTS; do
        run_on $host "sudo kubeadm reset --force && sudo rm -r -f /opt/cni/bin/*weave*"
        # Remove the rule to ensure that api-server is not accessible while kube-proxy
        # is not ready. Otherwise, we have a race between "-j KUBE-FORWARD" and "-j WEAVE-NPC"
        # which makes the NodePort isolation test case to fail.
        run_on $host "sudo iptables -t nat -D PREROUTING -m comment --comment \"kubernetes service portals\" -j KUBE-SERVICES" || true
    done
}

howmany() { echo $#; }

start_suite "Test weave-kube image with Kubernetes"

TOKEN=112233.4455667788990000
HOST1IP=$($SSH $HOST1 "getent hosts $HOST1 | cut -f 1 -d ' '")
NUM_HOSTS=$(howmany $HOSTS)
SUCCESS="$(( $NUM_HOSTS * ($NUM_HOSTS-1) )) established"
KUBECTL="sudo kubectl --kubeconfig /etc/kubernetes/admin.conf"
KUBE_PORT=6443
WEAVE_NETWORK=10.32.0.0/12
IMAGE=weaveworks/network-tester:latest
DOMAIN=nettest.default.svc.cluster.local.

tear_down_kubeadm

# Make an ipset, so we can check it doesn't get wiped out by Weave Net
docker_on $HOST1 run --rm --privileged --net=host --entrypoint=/usr/sbin/ipset weaveworks/weave-npc create test_840_ipset bitmap:ip range 192.168.1.0/24 || true
docker_on $HOST1 run --rm --privileged --net=host --entrypoint=/usr/sbin/ipset weaveworks/weave-npc add test_840_ipset 192.168.1.11

# kubeadm init upgrades to latest Kubernetes version by default, therefore we try to lock the version using the below option:
k8s_version="$(run_on $HOST1 "kubelet --version" | grep -oP "(?<=Kubernetes )v[\d\.\-beta]+")"

for host in $HOSTS; do
    if [ $host = $HOST1 ] ; then
	run_on $host "sudo systemctl start kubelet && sudo kubeadm init --kubernetes-version=$k8s_version --token=$TOKEN --pod-network-cidr=$WEAVE_NETWORK"
    else
	run_on $host "sudo systemctl start kubelet && sudo kubeadm join --token=$TOKEN --discovery-token-unsafe-skip-ca-verification $HOST1IP:$KUBE_PORT"
    fi
done

if [ -n "$COVERAGE" ]; then
    WEAVE_ENV_VARS="env:\\n                - name: EXTRA_ARGS\\n                  value: \"-test.coverprofile=/home/weave/cover.prof --\""
else
    WEAVE_ENV_VARS="env:"
fi

# Ensure Kubernetes uses locally built container images and inject code coverage environment variable (or do nothing depending on $COVERAGE):
sed -e "s%imagePullPolicy: Always%imagePullPolicy: Never%" \
    -e "s%env:%$WEAVE_ENV_VARS%" \
    "$(dirname "$0")/../prog/weave-kube/weave-daemonset-k8s-1.9.yaml" | run_on "$HOST1" "$KUBECTL apply -n kube-system -f -"

sleep 5

check_connections() {
    run_on $HOST1 "curl -sS http://127.0.0.1:6784/status | grep \"$SUCCESS\""
}

assert_raises 'wait_for_x check_connections "connections to establish"'

# Check we can ping between the Weave bridge IPs on each host
HOST1EXPIP=$($SSH $HOST1 "weave expose" || true)
HOST2EXPIP=$($SSH $HOST2 "weave expose" || true)
HOST3EXPIP=$($SSH $HOST3 "weave expose" || true)
assert_raises "run_on $HOST1 $PING $HOST2EXPIP"
assert_raises "run_on $HOST2 $PING $HOST1EXPIP"
assert_raises "run_on $HOST3 $PING $HOST2EXPIP"

# Ensure we do not generate any defunct process (e.g. launch.sh) after starting weaver:
assert "run_on $HOST1 ps aux | grep -c '[d]efunct'" "0"
assert "run_on $HOST2 ps aux | grep -c '[d]efunct'" "0"
assert "run_on $HOST3 ps aux | grep -c '[d]efunct'" "0"

# Set up simple network policies so all our test pods can talk to each other
run_on $HOST1 "$KUBECTL apply -f -" <<EOF
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: default-deny
spec:
  podSelector: {}
  policyTypes:
  - Ingress
EOF
run_on $HOST1 "$KUBECTL apply -f -" <<EOF
apiVersion: networking.k8s.io/v1
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

# Another policy, this time with no 'from' section, just to check that doesn't cause a crash
run_on $HOST1 "$KUBECTL apply -f -" <<EOF
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: test840f
spec:
  ingress:
  - {}
  podSelector:
    matchLabels:
      run: norealpods
EOF

check_ready() {
    run_on $HOST1 "$KUBECTL get nodes | grep -c -w Ready | grep $NUM_HOSTS"
}

assert_raises 'wait_for_x check_ready "hosts to be ready"'

# See if we can get some pods running that connect to the network
run_on $HOST1 "$KUBECTL run --image-pull-policy=Never nettest --image=$IMAGE --replicas=3 -- -peers=3 -dns-name=$DOMAIN"
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

# And a NodePort service so we can test virtual IP access
run_on $HOST1 "$KUBECTL create -f -" <<EOF
apiVersion: v1
kind: Service
metadata:
  name: netvirt
spec:
  type: NodePort
  ports:
  - port: 80
    targetPort: 8080
    nodePort: 31138
  selector:
    run: nettest
EOF

podName=$($SSH $HOST1 "$KUBECTL get pods -l run=nettest -o go-template='{{(index .items 0).metadata.name}}'")

check_all_pods_communicate() {
    if [ -n podIP ] ; then
        status=$($SSH $HOST1 "$KUBECTL exec $podName -- curl -s -S http://127.0.0.1:8080/status")
        if [ $status = "pass" ] ; then
            return 0
        fi
    fi
    return 1
}

assert_raises 'wait_for_x check_all_pods_communicate pods'

# Check that a pod can contact the outside world
assert_raises "$SSH $HOST1 $KUBECTL exec $podName -- $PING 8.8.8.8"

# Check that our pods haven't crashed
assert "$SSH $HOST1 $KUBECTL get pods -n kube-system -l name=weave-net | grep -c Running" 3

# Start pod which should not have access to nettest
run_on $HOST1 "$KUBECTL run nettest-deny --labels=\"access=deny,run=nettest-deny\" --image-pull-policy=Never --image=$IMAGE --replicas=1 --command -- sleep 3600"
denyPodName=$($SSH $HOST1 "$KUBECTL get pods -l run=nettest-deny -o go-template='{{(index .items 0).metadata.name}}'")
assert_raises "! $SSH $HOST1 $KUBECTL exec $denyPodName -- curl -s -S -f -m 2 http://$DOMAIN:8080/status >/dev/null"

# check access via virtual IP
VIRTUAL_IP=$($SSH $HOST1 $KUBECTL get service netvirt -o template --template={{.spec.clusterIP}})
assert_raises   "$SSH $HOST1 $KUBECTL exec     $podName -- curl -s -S -f -m 2 http://$VIRTUAL_IP/status >/dev/null"
assert_raises "! $SSH $HOST1 $KUBECTL exec $denyPodName -- curl -s -S -f -m 2 http://$VIRTUAL_IP/status >/dev/null"

# host should not be able to reach pods via service virtual IP or NodePort
assert_raises "! $SSH $HOST1 curl -s -S -f -m 2 http://$VIRTUAL_IP/status >/dev/null"
assert_raises "! $SSH $HOST1 curl -s -S -f -m 2 http://$HOST2:31138/status >/dev/null"

# allow access for nettest-deny
run_on $HOST1 "$KUBECTL apply -f -" <<EOF
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-nettest-deny
  namespace: default
spec:
  podSelector:
    matchLabels:
      run: nettest
  ingress:
    - from:
        - podSelector:
            matchLabels:
              access: deny
EOF

# Allow some time for the policy change to take effect
sleep 2

assert_raises "$SSH $HOST1 $KUBECTL exec $denyPodName -- curl -s -S -f -m 2 http://$DOMAIN:8080/status >/dev/null"

# remove the access for nettest-deny
run_on $HOST1 "$KUBECTL delete netpol allow-nettest-deny"

# Allow some time for the policy change to take effect
sleep 2

# nettest-deny should still not be able to reach nettest pods
assert_raises "! $SSH $HOST1 $KUBECTL exec $denyPodName -- curl -s -S -f -m 2 http://$DOMAIN:8080/status >/dev/null"

# Create many namespaces to stress namespaceSelector
for n in 1 2 3 4 5 6 7 8 9 10; do
    run_on $HOST1 "$KUBECTL create namespace namespace${n}"
done

# allow access from any namespace
run_on $HOST1 "$KUBECTL apply -f -" <<EOF
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-any-namespace
  namespace: default
spec:
  podSelector: {}
  ingress:
    - from:
      - namespaceSelector: {}
EOF

# Should be able to access from the "deny" pod now
assert_raises "$SSH $HOST1 $KUBECTL exec $denyPodName -- curl -s -S -f -m 2 http://$DOMAIN:8080/status >/dev/null"

# host should still not be able to reach pods via service virtual IP or NodePort
# because host is not in a namespace
assert_raises "! $SSH $HOST1 curl -s -S -f -m 2 http://$VIRTUAL_IP/status >/dev/null"
assert_raises "! $SSH $HOST1 curl -s -S -f -m 2 http://$HOST2:31138/status >/dev/null"

# allow access only from weave bridge IP of host1 to test ipBlock
run_on $HOST1 "$KUBECTL apply -f -" <<EOF
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-via-weave-bridge-of-host1
  namespace: default
spec:
  podSelector: {}
  ingress:
    - from:
      - ipBlock:
          cidr: $HOST1EXPIP/32
EOF

sleep 2

assert_raises "$SSH $HOST1 curl -s -S -f -m 2 http://$VIRTUAL_IP/status >/dev/null"

run_on $HOST1 "$KUBECTL delete netpol allow-via-weave-bridge-of-host1"

# allow access from anywhere
run_on $HOST1 "$KUBECTL apply -f -" <<EOF
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-anywhere
  namespace: default
spec:
  podSelector: {}
  ingress:
    - {}
EOF

# Virtual IP and NodePort should now work
assert_raises "$SSH $HOST1 curl -s -S -f -m 2 http://$VIRTUAL_IP/status >/dev/null"
assert_raises "$SSH $HOST1 curl -s -S -f -m 2 http://$HOST2:31138/status >/dev/null"

# allow nettest-deny to access only to 8.8.8.0/24
run_on $HOST1 "$KUBECTL apply -f -" <<EOF
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: egress-nettest-deny-dns
  namespace: default
spec:
  podSelector:
    matchLabels:
      run: nettest-deny
  policyTypes:
  - Egress
  egress:
  - to:
    - ipBlock:
        cidr: 8.8.8.0/24
        except:
        - 8.8.8.4/32
EOF

assert_raises "! $SSH $HOST1 $KUBECTL exec $denyPodName -- curl -s -S -f -m 2 http://$DOMAIN:8080/status >/dev/null"
assert_raises "! $SSH $HOST1 $KUBECTL exec $denyPodName -- dig @8.8.8.4 google.com >/dev/null"
assert_raises "$SSH $HOST1 $KUBECTL exec $denyPodName -- dig @8.8.8.8 google.com >/dev/null"

# also, allow nettest-deny to access nettest and kube-system (for kube-dns)
run_on $HOST1 "$KUBECTL label namespace kube-system name=kube-system"
run_on $HOST1 "$KUBECTL replace -f -" <<EOF
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: egress-nettest-deny-dns
  namespace: default
spec:
  podSelector:
    matchLabels:
      run: nettest-deny
  policyTypes:
  - Egress
  egress:
  - to:
    - ipBlock:
        cidr: 8.8.8.0/24
        except:
        - 8.8.8.4/32
    - podSelector:
        matchLabels:
          run: nettest
    - namespaceSelector:
        matchLabels:
          name: kube-system
EOF

assert_raises "$SSH $HOST1 $KUBECTL exec $denyPodName -- curl -s -S -f -m 2 http://$DOMAIN:8080/status >/dev/null"

# Passing --no-masq-local and setting externalTrafficPolicy to Local must preserve
# the client source IP addr

CLIENT_IP_MASQ="$($SSH $HOST1 curl -sS http://$HOST2:31138/client_ip)"

WEAVE_ENV_VARS="${WEAVE_ENV_VARS}\\n                - name: NO_MASQ_LOCAL\\n                  value: \"1\""
$SSH $HOST1 "$KUBECTL delete ds weave-net -n=kube-system"
sed -e "s%imagePullPolicy: Always%imagePullPolicy: Never%" \
    -e "s%env:%$WEAVE_ENV_VARS%" \
    "$(dirname "$0")/../prog/weave-kube/weave-daemonset-k8s-1.9.yaml" | run_on "$HOST1" "$KUBECTL apply -n kube-system -f -"

sleep 5

run_on $HOST1 "$KUBECTL patch svc netvirt -p '{\"spec\":{\"externalTrafficPolicy\":\"Local\"}}'"

CLIENT_IP_NO_MASQ="$($SSH $HOST1 curl -sS http://$HOST2:31138/client_ip)"

assert_raises "[ $CLIENT_IP_NO_MASQ != $CLIENT_IP_MASQ ]"
assert_raises "[ $HOST1IP == $CLIENT_IP_NO_MASQ ]"

tear_down_kubeadm

# Destroy our test ipset, and implicitly check it is still there
assert_raises "docker_on $HOST1 run --rm --privileged --net=host --entrypoint=/usr/sbin/ipset weaveworks/weave-npc destroy test_840_ipset"

end_suite
