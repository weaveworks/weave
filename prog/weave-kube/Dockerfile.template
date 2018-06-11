FROM DOCKERHUB_USER/weaveARCH_EXT

# These labels are pretty static, and can therefore be added early on:
LABEL maintainer="Weaveworks <help@weave.works>" \
      org.opencontainers.image.title="weave-kube" \
      org.opencontainers.image.source="https://github.com/weaveworks/weave" \
      org.opencontainers.image.vendor="Weaveworks"

ADD ./launch.sh ./kube-utils /home/weave/
ENTRYPOINT ["/home/weave/launch.sh"]

# This label will change for every build, and should therefore be the last layer of the image:
ARG revision
LABEL org.opencontainers.image.revision="${revision}"
