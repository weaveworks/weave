FROM gliderlabs/alpine
MAINTAINER Weaveworks Inc <help@weave.works>
WORKDIR /home/weave
RUN ["apk", "add", "--update", "ethtool", "conntrack-tools", "curl", "iptables", "iproute2", "docker"]
RUN ["sh", "-c", "rm -rf /var/cache/apk/*"]
ADD ./weave /home/weave/
ENTRYPOINT ["/home/weave/weave"]
