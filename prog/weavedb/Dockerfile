# This is a nearly-empty image that we use to create a data-only container for persistence
FROM scratch
LABEL maintainer "Weaveworks Inc <help@weave.works>"
LABEL works.weave.role=system
ENTRYPOINT ["data-only"]
# Work round Docker refusing to save an empty image
COPY Dockerfile /
