FROM ubuntu

# When doing a build in a container, "apt-get update" happens twice,
# which can be a very significant overhead for incremental builds.
# And it's unnecessary, because if any of the prerequisites do change,
# the right thing to do is to rebuild the container image.  Rather
# than suppressing the "apt-get update" commands explicitly, we clear
# out sources.list so that 'apt-get update' doesn't do anything.

RUN sh -c 'apt-get -y update && apt-get -y install --no-install-recommends build-essential git ca-certificates golang docker.io mercurial libpcap-dev curl make pkg-config gcc bison flex && echo >/etc/apt/sources.list'
COPY build.sh /
ENTRYPOINT ["sh", "/build.sh"]
