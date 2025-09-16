# syntax=docker/dockerfile:1
ARG ALPINE_BASEIMAGE=alpine:3.20.3

########
# Stage 1: go 1.23.1 on debian bullseye
# This installs cross-compilation tools for supported architectures.
# It will be built only once for a given build platform. Subsequent
# stages will start from this and cross-compile for each target
# architecture.
FROM --platform=${BUILDPLATFORM} golang:1.23.1-bullseye AS builderbase

# Support Raspberry Pi 2 and newer
ENV GOARM 7

# The names of the architectures that we fetch cross-compilers for
# from the Debian packages.
# Disabling support for ppc64el and s390x architectures, as the 
# debian buster repository does not host binaries for libpcap-0.8
# on those architectures any more.
# ENV DEB_CROSSPLATFORMS \
#       amd64 \
# 	armhf \
# 	arm64 \
# 	ppc64el \
# 	s390x
ENV DEB_CROSSPLATFORMS \
      amd64 \
	armhf \
	arm64

# Install the build-essential and crossbuild-essential-ARCH packages
RUN <<EOCROSSCOMPILERS
set -e
echo 'Acquire::Retries "6";' > /etc/apt/apt.conf.d/80-retries
for platform in ${DEB_CROSSPLATFORMS}; do dpkg --add-architecture $platform; done 
apt-get update
apt-get install -y build-essential
for platform in ${DEB_CROSSPLATFORMS}; do apt-get install -y crossbuild-essential-${platform}; done 
apt-get clean
rm -rf /var/lib/apt/lists/*
EOCROSSCOMPILERS

# weave net is dependent on libpcap, which it statically links.
# Later versions of libpcap do not allow static linking. The
# last version in stable Debian repositories which does is 1.8.1
# , which is available in the debian buster repos.
# Add buster repositories
# Install libpcap-dev version 1.8.1-6+deb10u1 for all supported
# architectures.
RUN <<EOBUSTERREPOS
echo "deb http://deb.debian.org/debian/ buster main" > /etc/apt/sources.list.d/busters-repos.list
echo "deb-src http://deb.debian.org/debian/ buster main" >> /etc/apt/sources.list.d/busters-repos.list
apt-get update
for platform in ${DEB_CROSSPLATFORMS}; do apt-get install -y libpcap0.8:${platform}=1.8.1-6+deb10u1 libpcap0.8-dev:${platform}=1.8.1-6+deb10u1 libpcap-dev:${platform}=1.8.1-6+deb10u1; done
EOBUSTERREPOS
# End Stage 1
########

########
# Stage 2: Compile weave-net executables.
# This stage builds one image per supported architecture. The
# companion Makefile compiles all executables that make up weave
# net, for the target architecture. Subsequent stages each build
# an image for a single component of weave-net.
FROM builderbase AS builder
ARG BUILDARCH
ARG TARGETARCH
ARG WEAVE_VERSION
ENV ARCH=${TARGETARCH}
ENV BUILDARCH=${BUILDARCH}
# Git hash that will be built into executables
ENV WEAVE_VERSION=${WEAVE_VERSION}

WORKDIR /root/weave
COPY . .

RUN go mod download -x
RUN <<EOMAKE
cd reweave/build 
CGO_ENABLED=1 make all
EOMAKE
# End Stage 2
########

########
# Stage 3: Alpine Base
# This stage exists in case there are any additional tweaks 
# required in the alpine base image.
FROM --platform=linux/${TARGETARCH} ${ALPINE_BASEIMAGE} AS alpinebase
# Any additional tweaks will appear here
# Last update on top of alpine:3.19.1
RUN apk update \
    && apk upgrade \
    && rm -rf /var/cache/apk/*
# End Stage 3
########

########
# Stage 4: Weaver image
# This will be tagged as REGISTRY_USER/weave:VERSION
FROM alpinebase AS weaverimage

LABEL works.weave.role="system" \
      maintainer="Raj Chaudhuri <raj@rajware.net>" \
      org.opencontainers.image.title="Weave Net" \
      org.opencontainers.image.description="Weave Net creates a virtual network that connects Docker containers across multiple hosts and enables their automatic discovery" \
      org.opencontainers.image.url="https://github.com/rajch/weave" \
      org.opencontainers.image.source="https://github.com/rajch/weave" \
      org.opencontainers.image.vendor="Raj Chaudhuri (forked from Weaveworks)" \
      org.label-schema.schema-version="1.0" \
      org.label-schema.name="Weave Net" \
      org.label-schema.description="Weave Net creates a virtual network that connects Docker containers across multiple hosts and enables their automatic discovery" \
      org.label-schema.url="https://github.com/rajch/weave" \
      org.label-schema.vcs-url="https://github.com/rajch/weave" \
      org.label-schema.vendor="Raj Chaudhuri (forked from Weaveworks)"

RUN <<EOWEAVER
      apk add --update \
                  curl \
                  iptables \
                  ipset \
                  iproute2 \
                  conntrack-tools \
                  bind-tools \
                  ca-certificates
      # Alpine 3.19 made nftables the default backend for iptables
      # From weave version 2.9.0 onwards, weave will also assume 
      # the same, but will include iptables-legacy for backward
      # compatibility.
      apk add iptables-legacy
      # ln -sf /sbin/iptables-legacy /sbin/iptables
      # ln -sf /sbin/iptables-legacy-save /sbin/iptables-save
      # ln -sf /sbin/iptables-legacy-restore /sbin/iptables-restore
      rm -rf /var/cache/apk/*
EOWEAVER

COPY --from=builder /root/weave/weave /root/weave/prog/weaver/weaver /home/weave/
COPY --from=builder /root/weave/prog/weaveutil/weaveutil /usr/bin/
COPY --from=builder /root/weave/prog/weaver/weavedata.db /weavedb/
ENTRYPOINT ["/home/weave/weaver"]
WORKDIR /home/weave

ARG revision
ARG imageversion
LABEL org.opencontainers.image.revision="${revision}" \
      org.label-schema.vcs-ref="${revision}" \
      org.reweave.image-version="${imageversion}"
# End Stage 4
########

########
# Stage 5: weaveexec image
# This will be tagged as REGISTRY_USER/weaveexec:VERSION
FROM weaverimage AS weavexecimage

LABEL maintainer="Raj Chaudhuri <raj@rajware.net>" \
      org.opencontainers.image.title="weaveexec" \
      org.opencontainers.image.source="https://github.com/rajch/weave" \
      org.opencontainers.image.vendor="Raj Chaudhuri (forked from Weaveworks)"

ENTRYPOINT ["/home/weave/sigproxy", "/home/weave/weave"]

COPY --from=builder /root/weave/prog/sigproxy/sigproxy /root/weave/prog/weaveexec/symlink /home/weave/
COPY --from=builder /root/weave/prog/weavewait/weavewait /w/w
COPY --from=builder /root/weave/prog/weavewait/weavewait_noop /w-noop/w
COPY --from=builder /root/weave/prog/weavewait/weavewait_nomcast /w-nomcast/w
WORKDIR /home/weave

ARG revision
ARG imageversion
LABEL org.opencontainers.image.revision="${revision}" \
      org.reweave.image-version="${imageversion}"
# End Stage 5
########

########
# Stage 6: weavekube image
# This will be tagged as REGISTRY_USER/weave-kube:VERSION
FROM weaverimage AS weavekubeimage

LABEL maintainer="Raj Chaudhuri <raj@rajware.net>" \
      org.opencontainers.image.title="weave-kube" \
      org.opencontainers.image.source="https://github.com/rajch/weave" \
      org.opencontainers.image.vendor="Raj Chaudhuri (forked from Weaveworks)"

COPY --from=builder /root/weave/prog/weave-kube/init.sh /root/weave/prog/weave-kube/launch.sh /root/weave/prog/kube-utils/kube-utils /home/weave/
ENTRYPOINT ["/home/weave/launch.sh"]

ARG revision
ARG imageversion
LABEL org.opencontainers.image.revision="${revision}" \
      org.reweave.image-version="${imageversion}"
# End Stage 6
########

########
# Stage 7: weavenpc image
# This will be tagged as REGISTRY_USER/weave-npc:VERSION
FROM alpinebase AS weavenpcimage

LABEL works.weave.role="system" \
      maintainer="Raj Chaudhuri <raj@rajware.net>" \
      org.opencontainers.image.title="Weave Net" \
      org.opencontainers.image.description="Weave Net creates a virtual network that connects Docker containers across multiple hosts and enables their automatic discovery" \
      org.opencontainers.image.url="https://github.com/rajch/weave" \
      org.opencontainers.image.source="https://github.com/rajch/weave" \
      org.opencontainers.image.vendor="Raj Chaudhuri (forked from Weaveworks)" \
      org.label-schema.schema-version="1.0" \
      org.label-schema.name="Weave Net" \
      org.label-schema.description="Weave Net creates a virtual network that connects Docker containers across multiple hosts and enables their automatic discovery" \
      org.label-schema.url="https://github.com/rajch/weave" \
      org.label-schema.vcs-url="https://github.com/rajch/weave" \
      org.label-schema.vendor="Raj Chaudhuri (forked from Weaveworks)"

RUN <<EONPC
      apk add --update \
                  iptables \
                  ipset \
                  ulogd
      mknod /var/log/ulogd.pcap p
      
      # Alpine 3.19 made nftables the default backend for iptables
      # From weave version 2.9.0 onwards, weave will also assume 
      # the same, but will include iptables-legacy for backward
      # compatibility.
      apk add iptables-legacy
      # ln -sf /sbin/iptables-legacy /sbin/iptables
      # ln -sf /sbin/iptables-legacy-save /sbin/iptables-save
      # ln -sf /sbin/iptables-legacy-restore /sbin/iptables-restore

      rm -rf /var/cache/apk/*
EONPC
COPY --from=builder /root/weave/prog/weave-npc/weave-npc /usr/bin/weave-npc
COPY --from=builder /root/weave/prog/weave-npc/ulogd.conf /etc/ulogd.conf
COPY --from=builder /root/weave/prog/weave-npc/launch.sh /usr/bin/
ENTRYPOINT ["/usr/bin/launch.sh"]

ARG revision
ARG imageversion
LABEL org.opencontainers.image.revision="${revision}" \
      org.label-schema.vcs-ref="${revision}" \
      org.reweave.image-version="${imageversion}"
# End Stage 7
########

########
# Stage 8: weavedb image
# This is a nearly-empty image that we use to create a data-only
# container for persistence.
# This will be tagged as REGISTRY_USER/weavedb:VERSION
FROM scratch AS weavedbimage

LABEL works.weave.role="system" \
      maintainer="Raj Chaudhuri <raj@rajware.net>" \
      org.opencontainers.image.title="weavedb" \
      org.opencontainers.image.source="https://github.com/rajch/weave" \
      org.opencontainers.image.vendor="Raj Chaudhuri (forked from Weaveworks)"

ENTRYPOINT ["data-only"]
# Workaround for Docker refusing to save an empty image
COPY --from=builder /etc/os-release /etc/os-release

ARG revision
ARG imageversion
LABEL org.opencontainers.image.revision="${revision}" \
      org.reweave.image-version="${imageversion}"
# End Stage 8
########

########
# Stage 9: Network tester
# This will be tagged as REGISTRY_USER/network-tester:VERSION
#
# Copyright 2016 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

FROM --platform=linux/${TARGETARCH} bash AS networktesterimage
RUN apk add --no-cache curl bind-tools
COPY --from=builder /root/weave/test/images/network-tester/webserver webserver
EXPOSE 8080
ENTRYPOINT ["/webserver"]
# End Stage 9
########
