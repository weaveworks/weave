FROM scratch
WORKDIR /home/weave
ADD ./weave /home/weave/
CMD []
ENTRYPOINT ["/home/weave/weave", "-iface", "ethwe", "-wait"]
EXPOSE 6783/tcp 6783/udp
