FROM scratch
MAINTAINER Weaveworks Inc <help@weave.works>
WORKDIR /home/weave
ADD ./weaver /home/weave/
ENTRYPOINT ["/home/weave/weaver", "-wait", "20"]
