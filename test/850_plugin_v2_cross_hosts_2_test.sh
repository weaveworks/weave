#! /bin/bash

. "$(dirname "$0")/config.sh"

PLUGIN_NAME="weave-ci-registry:5000/weaveworks/plugin-v2"
HOST1_IP=$($SSH $HOST1 getent ahosts $HOST1 | grep "RAW" | cut -d' ' -f1)

# TODO(mp) Add a Docker vsn check

setup_master() {
    # Setup Docker image registry on $HOST1
    docker_on $HOST1 run -p 5000:5000 -d registry:2

    # Build plugin-v2 on $HOST1, because Circle "CI" runs only ancient
    # version of Docker which does not support v2 plugins...
    BUILD_PLUGIN_IMG="buildpluginv2-ci"
    rsync -az -e "$SSH" "$(dirname $0)/../prog/plugin-v2" $HOST1:~/
    $SSH $HOST1<<EOF
        docker rmi $BUILD_PLUGIN_IMG 2>/dev/null
        docker plugin disable $PLUGIN_NAME >/dev/null
        docker plugin remove $PLUGIN_NAME 2>/dev/null

        WORK_DIR=$(mktemp -d)
        mkdir -p \${WORK_DIR}/rootfs

        docker create --name=$BUILD_PLUGIN_IMG weaveworks/plugin true
        docker export $BUILD_PLUGIN_IMG | tar -x -C \${WORK_DIR}/rootfs
        cp \${HOME}/plugin-v2/launch.sh \${WORK_DIR}/rootfs/home/weave/launch.sh
        cp \${HOME}/plugin-v2/config.json \${WORK_DIR}
        docker plugin create $PLUGIN_NAME \${WORK_DIR}

        echo "$HOST1_IP weave-ci-registry" | sudo tee -a /etc/hosts
        docker plugin push $PLUGIN_NAME
EOF

    # Start Swarm Manager and enable the plugin
    $SSH $HOST1<<EOF
        docker swarm init --advertise-addr=$HOST1_IP
        docker plugin enable $PLUGIN_NAME
EOF
}

setup_worker() {
    $SSH $HOST2<<EOF
        echo "$HOST1_IP weave-ci-registry" | sudo tee -a /etc/hosts
        docker swarm join --token "$1" "${HOST1_IP}:2377"
        docker plugin install --grant-all-permissions $PLUGIN_NAME
EOF
}

cleanup() {
    HOSTS="$HOST1 HOST2"
    for HOST in $HOSTS; do
        $SSH $HOST<<EOF
            sudo sed '/weave-ci-registry/d' /etc/hosts
            docker plugin disable $PLUGIN_NAME
            docker plugin remove $PLUGIN_NAME
            docker swarm leave --force
EOF
    done
}

start_suite "Test weave Docker plugin-v2"

setup_master
setup_worker $($SSH $HOST1 docker swarm join-token --quiet worker)

assert_raises "$SSH $HOST1 ping -nq -W 2 -c 1 weave-ci-registry"
assert_raises "$SSH $HOST2 ping -nq -W 2 -c 1 weave-ci-registry"

# Create network and service
$SSH $HOST1<<EOF
    docker network create --driver="${PLUGIN_NAME}:latest" weave-v2
    docker service create --name=weave1 --network=weave-v2 --replicas=2 nginx
EOF

# TODO(mp) add wait for
sleep 10

# TODO(mp) ...
C1=$(SSH $HOST2 weave ps | grep -v weave:expose | awk '{print $1}')
C2_IP=$($SSH $HOST2 weave ps | grep -v weave:expose | awk '{print $3}')

assert_raises "exec_on $HOST1 $C1 $PING $C2_IP"

cleanup

end_suite
