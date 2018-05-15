FROM DOCKERHUB_USER/weaveARCH_EXT

# These labels are pretty static, and can therefore be added early on:
LABEL maintainer="Weaveworks <help@weave.works>" \
      org.opencontainers.image.title="weaveexec" \
      org.opencontainers.image.source="https://github.com/weaveworks/weave" \
      org.opencontainers.image.vendor="Weaveworks"

ENTRYPOINT ["/home/weave/sigproxy", "/home/weave/weave"]

ADD ./sigproxy ./symlink /home/weave/
ADD ./weavewait /w/w
ADD ./weavewait_noop /w-noop/w
ADD ./weavewait_nomcast /w-nomcast/w
WORKDIR /home/weave

# This label will change for every build, and should therefore be the last layer of the image:
ARG revision
LABEL org.opencontainers.image.revision="${revision}"
