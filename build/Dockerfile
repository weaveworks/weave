FROM ubuntu:15.04

RUN apt-get -y update && apt-get -y install --no-install-recommends ca-certificates apt-transport-https
RUN apt-key adv --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys 36A1D7869245C8950F966E92D8576A8BA88D21E9
RUN echo deb https://get.docker.io/ubuntu docker main > /etc/apt/sources.list.d/docker.list
RUN apt-get -y update && apt-get -y install --no-install-recommends build-essential git lxc-docker-1.3.1 mercurial libpcap-dev curl make pkg-config gcc bison flex python-requests sudo

# When doing a build in a container, "apt-get update" happens twice,
# which can be a very significant overhead for incremental builds.
# And it's unnecessary, because if any of the prerequisites do change,
# the right thing to do is to rebuild the container image.  Rather
# than suppressing the "apt-get update" commands explicitly, we clear
# out sources.list so that 'apt-get update' doesn't do anything.
RUN echo >/etc/apt/sources.list

ENV GO_VERSION 1.4.2
RUN curl -sSL https://golang.org/dl/go${GO_VERSION}.linux-amd64.tar.gz | tar -C /usr/local -xz
ENV PATH /usr/local/go/bin:$PATH

# So that sudo does not trample on PATH
RUN sed -ie '/secure_path/d' /etc/sudoers

COPY build.sh /
ENTRYPOINT ["sh", "/build.sh"]
