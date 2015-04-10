#!/bin/sh
set -e

GOPATH=/home/go
export GOPATH

WEAVE_SRC=$GOPATH/src/github.com/weaveworks/weave

if [ $# -eq 0 ] ; then
    # No arguments.  Expect that the weave repo will be bind-mounted
    # into $GOPATH
    if ! [ -e $WEAVE_SRC ] ; then
        cat 2>&1 <<EOF
No container arguments supplied, and nothing at ${WEAVE_SRC}.  Please
either bind-mount the golang workspace containing weave with the
docker run -v option, e.g.:

    $ docker run -v <host gopath>:${GOPATH} \\
          -v /var/run/docker.sock:/var/run/docker.sock weaveworks/weave-build

Or supply git clone arguments to retrieve it, e.g.:

    $ docker run -v /var/run/docker.sock:/var/run/docker.sock \\
          weaveworks/weave-build https://github.com/weaveworks/weave.git
EOF
        exit 1
    fi

    # If we run make directly, any files created on the bind mount
    # will have awkward ownership.  So we switch to a user with the
    # same user and group IDs as source directory.  We have to set a
    # few things up so that sudo works without complaining later on.
    uid=$(stat --format="%u" $WEAVE_SRC)
    gid=$(stat --format="%g" $WEAVE_SRC)
    echo "weave:x:$uid:$gid::$WEAVE_SRC:/bin/sh" >>/etc/passwd
    echo "weave:*:::::::" >>/etc/shadow
    echo "weave	ALL=(ALL)	NOPASSWD: ALL" >>/etc/sudoers
    su weave -c "PATH=$PATH make -C $WEAVE_SRC build"
else
    # There are arguments to pass to git-clone
    mkdir -p ${WEAVE_SRC%/*}
    git clone "$@" $WEAVE_SRC
    make -C $WEAVE_SRC build
fi
