FROM scratch
MAINTAINER Weaveworks Inc <help@weave.works>
WORKDIR /home/weave
ADD ./weavediscovery /home/weave/
EXPOSE 6789/tcp
ENTRYPOINT ["/home/weave/weavediscovery"]
