FROM gliderlabs/alpine

MAINTAINER Weaveworks Inc <help@weave.works>

WORKDIR /home/weave
VOLUME /w
ENTRYPOINT ["/home/weave/sigproxy", "/home/weave/weave"]

RUN apk add --update \
    curl \
    ethtool \
    iptables \
    iproute2 \
    util-linux \
    conntrack-tools \
    bind-tools \
  && rm -rf /var/cache/apk/*

ADD ./weave ./sigproxy ./weaveproxy /home/weave/
ADD ./netcheck /usr/bin/
ADD ./weavewait /w/w
ADD ./docker.tgz /
