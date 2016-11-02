FROM alpine
RUN apk add --update \
    iptables \
    ipset \
	ulogd \
  && rm -rf /var/cache/apk/* \
  && mknod /var/log/ulogd.pcap p
COPY ./weave-npc /usr/bin/weave-npc
COPY ./ulogd.conf /etc/ulogd.conf
ENTRYPOINT ["/usr/bin/weave-npc"]
