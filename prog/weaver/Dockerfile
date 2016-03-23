FROM gliderlabs/alpine
MAINTAINER Weaveworks Inc <help@weave.works>
LABEL works.weave.role=system
RUN apk-install ca-certificates
WORKDIR /home/weave
ADD ./weaver /home/weave/
ADD weavedata.db /weavedb/
COPY ca-certificates.crt /etc/ssl/certs/
ENTRYPOINT ["/home/weave/weaver"]
