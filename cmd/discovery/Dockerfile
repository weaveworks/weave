FROM scratch
MAINTAINER Weaveworks Inc <help@weave.works>
WORKDIR /home/weave
ADD ./weavediscovery /home/weave/
ENTRYPOINT ["/home/weave/weavediscovery", "-debug"]
