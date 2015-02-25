#!/bin/bash

set -e

if ! [ -f "./assert.sh" ]; then
    echo Fetching assert script
    curl -sS https://raw.githubusercontent.com/lehmannro/assert.sh/master/assert.sh > ./assert.sh
fi

. ./config.sh

echo Copying weave images and script to hosts
for HOST in $HOSTS; do
    docker_on $HOST load -i ../weave.tar
    docker_on $HOST load -i ../weavedns.tar
    docker_on $HOST load -i ../weaveexec.tar
    run_on $HOST mkdir -p `dirname $WEAVE`
    cat ../weave         | run_on $HOST sh -c "cat > $WEAVE"
    cat ../bin/docker-ns | run_on $HOST sh -c "cat > $DOCKER_NS"
    run_on $HOST chmod a+x $WEAVE
    run_on $HOST chmod a+x $DOCKER_NS
    run_on $HOST sudo service docker restart
done
