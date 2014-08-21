FROM scratch
WORKDIR /home/weave
ADD ./weaver /home/weave/
CMD []
ENTRYPOINT ["/home/weave/weaver", "-iface", "ethwe", "-wait"]
EXPOSE 6783/tcp 6783/udp
