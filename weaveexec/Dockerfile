FROM gliderlabs/alpine
MAINTAINER Weaveworks Inc <help@weave.works>
WORKDIR /home/weave
RUN ["apk", "add", "--update", "ethtool", "conntrack-tools", "curl", "iptables", "iproute2"]
RUN ["sh", "-c", "rm -rf /var/cache/apk/*"]
ADD ./weave /home/weave/
ADD ./docker.tgz /
ENTRYPOINT ["/home/weave/weave"]
