FROM gliderlabs/alpine
MAINTAINER Weaveworks Inc <help@weave.works>
WORKDIR /home/weave
RUN ["apk", "add", "--update", "ethtool", "conntrack-tools", "curl", "iptables", "iproute2", "util-linux"]
RUN ["sh", "-c", "rm -rf /var/cache/apk/*"]
ADD ./weave /home/weave/
ADD ./sigproxy /home/weave/
ADD ./proxy /home/weave/
ADD ./docker.tgz /
ADD ./wait-for-weave /home/weavewait/wait-for-weave
VOLUME /home/weavewait
ENTRYPOINT ["/home/weave/sigproxy", "/home/weave/weave"]
