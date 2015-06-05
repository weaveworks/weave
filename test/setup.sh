#!/bin/bash

set -e

. ./config.sh

(cd ./tls && go get ./... && go run generate_certs.go $HOSTS)

echo Copying weave images, scripts, and certificates to hosts
for HOST in $HOSTS; do
    docker_on $HOST load -i ../weave.tar
    docker_on $HOST load -i ../weavedns.tar
    docker_on $HOST load -i ../weaveexec.tar
    run_on $HOST mkdir -p bin
    cat ../bin/docker-ns | run_on $HOST sh -c "cat > $DOCKER_NS"
    cat ../weave | run_on $HOST sh -c "cat > ./weave"
    run_on $HOST chmod a+x $DOCKER_NS ./weave
    rsync -az -e "$SSH" ./tls/ $HOST:~/tls
    containers=$(docker_on $host ps -aq 2>/dev/null)
    [ -n "$containers" ] && docker_on $host rm -f $containers >/dev/null 2>&1
    run_on $HOST sudo service docker restart
done
