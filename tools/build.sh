#!/bin/sh
set -x -e

apt-get -y update
apt-get -y install curl make pkg-config gcc bison flex

BASEDIR=$(dirname $0)
mkdir -p "$BASEDIR"/bin

copy_exe() {
    if ldd $1 >/dev/null 2>&1 ; then
        echo "ERROR: $1 was not linked statically." 2>&1
        exit 1
    fi
    cp $1 "$BASEDIR"/bin/
}

# ethtool

ETHTOOL=ethtool-3.16

rm -rf $ETHTOOL

curl -s -S https://www.kernel.org/pub/software/network/ethtool/$ETHTOOL.tar.gz | tar xvz
(cd $ETHTOOL; ./configure LDFLAGS=-static && make)
copy_exe $ETHTOOL/ethtool

# conntrack

PACKAGES="libmnl-1.0.3 libnfnetlink-1.0.1 libnetfilter_cttimeout-1.0.0 libnetfilter_cthelper-1.0.0 libnetfilter_queue-1.0.2 libnetfilter_conntrack-1.0.4"
CONNTRACK=conntrack-tools-1.4.2

rm -rf $PACKAGES $CONNTRACK

fetch() {
    curl -s -S http://www.netfilter.org/projects/${1%-*}/files/$1.tar.bz2 | tar xj
}

for PACKAGE in $PACKAGES; do
    fetch $PACKAGE
    (cd $PACKAGE; ./configure --disable-shared && make install)
done

fetch $CONNTRACK
(cd $CONNTRACK; ./configure --disable-shared && make && rm -f src/conntrack && make LDFLAGS=-all-static)
copy_exe $CONNTRACK/src/conntrack

# curl

CURL=curl-7.40.0

rm -rf $CURL

curl -s -S  http://curl.haxx.se/download/$CURL.tar.gz | tar xvz
(cd $CURL; ./configure --without-ssl --disable-shared && make && rm src/curl && make LDFLAGS=-all-static)
copy_exe $CURL/src/curl

#

touch "$BASEDIR"/bin
