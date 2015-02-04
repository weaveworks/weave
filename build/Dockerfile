FROM ubuntu
# After installing the prerequisite we clear out sources.list so that
# 'apt-get update' doesn't do anything.
RUN sh -c 'apt-get -y update && apt-get -y install --no-install-recommends build-essential git ca-certificates golang docker.io mercurial libpcap-dev curl make pkg-config gcc bison flex && echo >/etc/apt/sources.list'
COPY build.sh /
ENTRYPOINT ["sh", "/build.sh"]
