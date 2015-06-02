FROM scratch
MAINTAINER Weaveworks Inc <help@weave.works>
WORKDIR /home/weave
ADD ./weavedns /home/weave/
EXPOSE 53/udp
ENTRYPOINT ["/home/weave/weavedns", "-wait", "20"]
