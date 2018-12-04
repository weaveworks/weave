# This is a nearly-empty image that we use to create a data-only container for persistence
FROM scratch

# These labels are pretty static, and can therefore be added early on:
LABEL works.weave.role="system" \
      maintainer="Weaveworks <help@weave.works>" \
      org.opencontainers.image.title="weavedb" \
      org.opencontainers.image.source="https://github.com/weaveworks/weave" \
      org.opencontainers.image.vendor="Weaveworks"

ENTRYPOINT ["data-only"]
# Work round Docker refusing to save an empty image
COPY Dockerfile /

# This label will change for every build, and should therefore be the last layer of the image:
ARG revision
LABEL org.opencontainers.image.revision="${revision}"
