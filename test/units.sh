#!/bin/bash

go get github.com/weaveworks/weave/testing/cover

if [ -n "$COVERDIR" ] ; then
	coverdir="$COVERDIR"
else
	coverdir=$(mktemp -d coverage.XXXXXXXXXX)
fi

mkdir -p $coverdir
fail=0

for dir in $(find . -type f -name '*_test.go' | xargs -n1 dirname | grep -v prog | sort -u); do
	go get -t -tags netgo $dir
	output=$(mktemp $coverdir/unit.XXXXXXXXXX)
	if ! go test -cpu 4 -tags netgo -covermode=count -coverprofile=$output $dir ; then
		fail=1
	fi
done

if [ -z "$COVERDIR" ] ; then
	cover $coverdir/* >profile.cov
	rm -rf $coverdir
	go tool cover -html=profile.cov -o=coverage.html
fi

exit $fail
