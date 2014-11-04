FROM scratch
# EXPOSE 53/udp
WORKDIR /home/weave
ADD ./weavedns /home/weave/
ENTRYPOINT ["/home/weave/weavedns", "-wait", "5", "-iface", "ethwe"]
