#!/bin/bash

set -e

echo Fetching assert script
curl -sS https://raw.githubusercontent.com/lehmannro/assert.sh/master/assert.sh > ./assert.sh

. ./config.sh

echo Copying weave images and script to hosts
for HOST in $HOST1 $HOST2; do
    docker_on $HOST load -i /var/tmp/weave.tar
    docker_on $HOST load -i /var/tmp/weavedns.tar
    run_on $HOST mkdir -p `dirname $WEAVE`
    cat ../weave         | run_on $HOST sh -c "cat > $WEAVE"
    cat ../bin/docker-ns | run_on $HOST sh -c "cat > $DOCKER_NS"
    run_on $HOST chmod a+x $WEAVE
    run_on $HOST chmod a+x $DOCKER_NS
    run_on $HOST sudo service docker restart
done
