FROM alpine

LABEL maintainer "Weaveworks Inc <help@weave.works>"
LABEL works.weave.role=system \
      org.label-schema.name="Weave Net" \
      org.label-schema.description="Weave Net creates a virtual network that connects Docker containers across multiple hosts and enables their automatic discovery" \
      org.label-schema.url="https://weave.works" \
      org.label-schema.vcs-url="https://github.com/weaveworks/weave" \
      org.label-schema.vendor="Weaveworks"

WORKDIR /home/weave
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

ADD ./weave ./sigproxy ./weaveproxy ./symlink /home/weave/
ADD ./weaveutil /usr/bin/
ADD ./weavewait /w/w
ADD ./weavewait_noop /w-noop/w
ADD ./weavewait_nomcast /w-nomcast/w
ADD ./docker.tgz /
