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
ADD ./proxy /home/weave/
ADD ./docker.tgz /

ADD ./wait-for-weave /home/weavewait/wait-for-weave
VOLUME /home/weavewait

ENTRYPOINT ["/home/weave/sigproxy", "/home/weave/weave"]
