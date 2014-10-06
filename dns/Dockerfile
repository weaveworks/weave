FROM ubuntu
MAINTAINER Zettio weave@zett.io
WORKDIR /home/weave
ADD ./dns /home/weave/
EXPOSE 53/udp
CMD ["/home/weave/dns"]
