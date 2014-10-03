FROM scratch
# EXPOSE 53/udp
WORKDIR /home/weave
ADD ./dns /home/weave/weavedns
ENTRYPOINT ["/home/weave/weavedns"]
