FROM gliderlabs/alpine

MAINTAINER Weaveworks Inc <help@weave.works>

RUN apk add --update \
    curl \
    ethtool \
    iptables \
    iproute2 \
    util-linux \
    conntrack-tools \
  && rm -rf /var/cache/apk/*

WORKDIR /home/weave

ADD ./weave /home/weave/
ADD ./sigproxy /home/weave/
ADD ./weaveproxy /home/weave/
ADD ./netcheck /usr/bin/
ADD ./weavewait /w/w
ADD ./docker.tgz /

VOLUME /w

ENTRYPOINT ["/home/weave/sigproxy", "/home/weave/weave"]
