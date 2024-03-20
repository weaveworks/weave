FROM golang:1.15.6-buster

# Support Raspberry Pi 2 and newer
ENV GOARM 7

# The names of the architectures that we're fetching cross-compilers for from the Debian packages
ENV DEB_CROSSPLATFORMS \
	armhf \
	arm64 \
	ppc64el \
	s390x

# The go platforms we are supporting to cross-compile
ENV GO_CROSSPLATFORMS \
	linux/arm \
	linux/arm64 \
	linux/ppc64le \
	linux/s390x

# The names of the gcc cross-compilers we have installed
ENV GCC_CROSSCOMPILERS \
	arm-linux-gnueabihf \
	aarch64-linux-gnu \
	powerpc64le-linux-gnu \
	s390x-linux-gnu

# Install the build-essential and crossbuild-essential-ARCH packages
RUN for platform in ${DEB_CROSSPLATFORMS}; do dpkg --add-architecture $platform; done \
  && apt-get update \
  && apt-get install -y build-essential \
  && for platform in ${DEB_CROSSPLATFORMS}; do apt-get install -y crossbuild-essential-${platform}; done \
  && apt-get clean && rm -rf /var/lib/apt/lists/*

# Install some required packages
# libpcap is required because we're linking against its C libraries from the prog/weaver binary
# flex and bison are required packages for compiling libpcap manually later
RUN apt-get update \
	&& apt-get install -y \
		libpcap-dev \
		python-requests \
		time \
		file \
		flex \
		bison

RUN curl -fsSLo shfmt https://github.com/mvdan/sh/releases/download/v1.3.0/shfmt_v1.3.0_linux_amd64 && \
	echo "b1925c2c405458811f0c227266402cf1868b4de529f114722c2e3a5af4ac7bb2  shfmt" | sha256sum -c && \
	chmod +x shfmt && \
	mv shfmt /usr/bin

# Install common Go tools
RUN go get \
    github.com/weaveworks/build-tools/cover \
    github.com/mattn/goveralls \
	golang.org/x/lint/golint \
	github.com/fzipp/gocyclo \
	github.com/fatih/hclfmt \
	github.com/client9/misspell/cmd/misspell

# Allow full write access to the Go folders for anyone
RUN chmod -R a+w /usr/local/go

# The libpcap version from Debian packages is 1.6.2, matching that version here although newer versions of libpcap have been released
# We have to cross-compile the libpcap library for the new architectures, that's what we're doing here
ENV LIBPCAP_CROSS_VERSION=1.6.2
RUN curl -sSL https://www.tcpdump.org/release/libpcap-${LIBPCAP_CROSS_VERSION}.tar.gz | tar -xz \
	&& cd libpcap-${LIBPCAP_CROSS_VERSION} \
	&& for crosscompiler in ${GCC_CROSSCOMPILERS}; do \
		CC=${crosscompiler}-gcc ac_cv_linux_vers=2 ./configure --host=${crosscompiler} --with-pcap=linux \
		&& make \
		&& export LIB_DIR="/usr/local/lib/${crosscompiler}-gcc" VER="$(cat ./VERSION)" MAJOR_VER="$(sed 's/\([0-9][0-9]*\)\..*/\1/' ./VERSION)" \
		&& mkdir -p ${LIB_DIR} \
		&& cp -f libpcap.a libpcap.so.${VER} ${LIB_DIR} \
		&& ln -sf libpcap.so.${VER} /usr/local/lib/libpcap.so.${MAJOR_VER} \
		&& ln -sf libpcap.so.${MAJOR_VER} /usr/local/lib/libpcap.so \
		&& make clean; done

# Install Docker
RUN curl -fsSL https://get.docker.com | VERSION=18.06.0-ce /bin/sh

COPY build.sh /
ENTRYPOINT ["sh", "/build.sh"]
