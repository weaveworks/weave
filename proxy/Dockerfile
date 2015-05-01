FROM scratch
MAINTAINER Weaveworks Inc <help@weave.works>
WORKDIR /home/weave
ADD ./proxy /home/weave/
ENTRYPOINT ["/home/weave/proxy", "-L", ":12375"]
CMD ["-H", "unix:///var/run/docker.sock"]
