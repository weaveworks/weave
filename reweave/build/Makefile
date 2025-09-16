.DEFAULT: all
.PHONY: all showbuild exes 

# This specifies the architecture we're building for, in GOARCH format
ARCH?=amd64

# This specifies the architecture we are building on , in GOARCH format
BUILDARCH?=amd64

# A list of all supported architectures here. Should be named as Go is
# naming platforms.
# All supported architectures must have an "ifeq" block below that 
# selects the corrsponding Debian platform architecture in CCARCH
# Disabling support for ppc64le and s390x architectures, as the 
# debian buster repository does not host binaries for libpcap-0.8
# on those architectures any more.
# ALL_ARCHITECTURES=amd64 arm arm64 ppc64le s390x
ALL_ARCHITECTURES=amd64 arm arm64

# We need the Debian platform architecture format, for
# crosscompilation
ifeq ($(ARCH),amd64)
	CCARCH=x86_64-linux-gnu
endif
ifeq ($(ARCH),arm)
	CCARCH=arm-linux-gnueabihf
endif
ifeq ($(ARCH),arm64)
	CCARCH=aarch64-linux-gnu
endif
# ifeq ($(ARCH),ppc64le)
# 	CCARCH=powerpc64le-linux-gnu
# endif
# ifeq ($(ARCH),s390x)
# 	CCARCH=s390x-linux-gnu
# endif

# Set the architecture-specific C compiler and library
# load path.
ifeq ($(ARCH),$(BUILDARCH))
	CC=gcc
	CGO_LDFLAGS=
else
	CC=$(CCARCH)-gcc
	CGO_LDFLAGS=-L/usr/lib/$(CCARCH)
endif

# The default version that's chosen when pushing the images. Can/should be overridden
#GIT_REVISION=$(shell git rev-parse HEAD)
# Git commit hash that will be built into executables
WEAVE_VERSION?=git-$(shell git rev-parse --short=12 HEAD)

# Paths to all relevant binaries that should be compiled
WEAVER_EXE=../../prog/weaver/weaver
SIGPROXY_EXE=../../prog/sigproxy/sigproxy
KUBEUTILS_EXE=../../prog/kube-utils/kube-utils
WEAVENPC_EXE=../../prog/weave-npc/weave-npc
WEAVEWAIT_EXE=../../prog/weavewait/weavewait
WEAVEWAIT_NOOP_EXE=../../prog/weavewait/weavewait_noop
WEAVEWAIT_NOMCAST_EXE=../../prog/weavewait/weavewait_nomcast
WEAVEUTIL_EXE=../../prog/weaveutil/weaveutil
RUNNER_EXE=../../tools/runner/runner
TEST_TLS_EXE=../../test/tls/tls
NETWORKTESTER_EXE=../../test/images/network-tester/webserver

# All binaries together in a list
EXES=$(WEAVER_EXE) $(SIGPROXY_EXE) $(KUBEUTILS_EXE) $(WEAVENPC_EXE) $(WEAVEWAIT_EXE) $(WEAVEWAIT_NOOP_EXE) $(WEAVEWAIT_NOMCAST_EXE) $(WEAVEUTIL_EXE) $(RUNNER_EXE) $(TEST_TLS_EXE) $(NETWORKTESTER_EXE)

# The flags we are passing to go build. -extldflags -static for making a static binary, 
# -X main.version for telling the Go binary which version it is,
# -s -w drop symbol tables used by debuggers (not Go's internal symbol info)
# -tags "osusergo netgo" to use native Go UID and DNS implementations
BUILD_FLAGS=-ldflags "-extldflags -static -X main.version=$(WEAVE_VERSION) -s -w" -tags "osusergo netgo"

# This allows adding up to three extra switches to the -ldflags value inside
# the same string returned by BUILD_FLAGS. Needed because the go # tool does
# not combine multiple -ldflags 
define extraldflags
-ldflags "-extldflags -static -X main.version=$(WEAVE_VERSION) $(strip $(1) $(2) $(3)) -s -w" -tags "osusergo netgo"
endef

# Targets
all: showbuild exes

showbuild:
	echo Build Arch: $(BUILDARCH)
	echo Target Arch: $(ARCH)
	echo CC: $(CC)
	echo CGO_LDFLAGS: $(CGO_LDFLAGS)

$(WEAVER_EXE) $(WEAVEUTIL_EXE): ../../common/*.go ../../common/*/*.go ../../net/*.go ../../net/*/*.go
$(WEAVER_EXE): ../../router/*.go ../../ipam/*.go ../../ipam/*/*.go ../../db/*.go ../../nameserver/*.go ../../prog/weaver/*.go
$(WEAVER_EXE): ../../api/*.go ../../plugin/*.go ../../plugin/*/*
$(WEAVER_EXE):  ../../proxy/*.go
$(WEAVEUTIL_EXE): ../../prog/weaveutil/*.go ../../net/*.go ../../plugin/net/*.go ../../plugin/ipam/*.go ../../db/*.go
$(SIGPROXY_EXE): ../../prog/sigproxy/*.go
$(KUBEUTILS_EXE): ../../prog/kube-utils/*.go
$(WEAVENPC_EXE): ../../prog/weave-npc/*.go ../../npc/*.go ../../npc/*/*.go
$(TEST_TLS_EXE): ../../test/tls/*.go
$(RUNNER_EXE): ../../tools/runner/*.go
$(WEAVEWAIT_NOOP_EXE): ../../prog/weavewait/*.go
$(WEAVEWAIT_EXE): ../../prog/weavewait/*.go ../../net/*.go
$(WEAVEWAIT_NOMCAST_EXE): ../../prog/weavewait/*.go ../../net/*.go
$(NETWORKTESTER_EXE): ../../test/images/network-tester/*.go

exes: $(EXES) 

$(WEAVER_EXE):
	CC=$(CC) GOOS=linux GOARCH=$(ARCH) go build $(BUILD_FLAGS) -o $@ ./$(@D)

$(WEAVEUTIL_EXE):
	CC=$(CC) GOOS=linux GOARCH=$(ARCH) go build $(call extraldflags,-X github.com/containernetworking/plugins/pkg/utils/buildversion.BuildVersion=$(WEAVE_VERSION)) -o $@ ./$(@D)

$(KUBEUTILS_EXE) $(WEAVENPC_EXE) $(NETWORKTESTER_EXE):
	CC=$(CC) GOOS=linux GOARCH=$(ARCH) go build $(BUILD_FLAGS) -o $@ ./$(@D)

$(WEAVEWAIT_EXE):
	CC=$(CC) GOOS=linux GOARCH=$(ARCH) go build $(BUILD_FLAGS) -tags "osusergo netgo iface mcast" -o $@ ./$(@D)

$(WEAVEWAIT_NOMCAST_EXE):
	CC=$(CC) GOOS=linux GOARCH=$(ARCH) go build $(BUILD_FLAGS) -tags "osusergo netgo iface" -o $@ ./$(@D)

# These programs need a separate rule as they fail the netgo check in
# the main build stanza due to not importing net package
$(SIGPROXY_EXE) $(TEST_TLS_EXE) $(WEAVEWAIT_NOOP_EXE) $(RUNNER_EXE):
	CC=$(CC) GOOS=linux GOARCH=$(ARCH) go build $(BUILD_FLAGS) -o $@ ./$(@D)
