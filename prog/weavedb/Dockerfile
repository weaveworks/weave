# This is a nearly-empty image that we use to create a data-only container for persistence
FROM scratch
MAINTAINER Weaveworks Inc <help@weave.works>
ENTRYPOINT ["data-only"]
# Work round Docker refusing to save an empty image
COPY Dockerfile /
