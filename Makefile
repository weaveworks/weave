PUBLISH=publish_weave publish_weaveexec publish_plugin publish_weave-kube

.DEFAULT: all
.PHONY: all exes testrunner update tests lint publish $(PUBLISH) clean clean-bin prerequisites build run-smoketests

# If you can use docker without being root, you can do "make SUDO="
SUDO=$(shell docker info >/dev/null 2>&1 || echo "sudo -E")
BUILD_IN_CONTAINER=true
RM=--rm
RUN_FLAGS=-ti
COVERAGE=

DOCKERHUB_USER=weaveworks
WEAVE_VERSION=git-$(shell git rev-parse --short=12 HEAD)

WEAVER_EXE=prog/weaver/weaver
WEAVEPROXY_EXE=prog/weaveproxy/weaveproxy
SIGPROXY_EXE=prog/sigproxy/sigproxy
KUBEPEERS_EXE=prog/kube-peers/kube-peers
WEAVEWAIT_EXE=prog/weavewait/weavewait
WEAVEWAIT_NOOP_EXE=prog/weavewait/weavewait_noop
WEAVEWAIT_NOMCAST_EXE=prog/weavewait/weavewait_nomcast
WEAVEUTIL_EXE=prog/weaveutil/weaveutil
PLUGIN_EXE=prog/plugin/plugin
RUNNER_EXE=tools/runner/runner
TEST_TLS_EXE=test/tls/tls

EXES=$(WEAVER_EXE) $(SIGPROXY_EXE) $(KUBEPEERS_EXE) $(WEAVEPROXY_EXE) $(WEAVEWAIT_EXE) $(WEAVEWAIT_NOOP_EXE) $(WEAVEWAIT_NOMCAST_EXE) $(WEAVEUTIL_EXE) $(PLUGIN_EXE) $(TEST_TLS_EXE)

BUILD_UPTODATE=.build.uptodate
WEAVER_UPTODATE=.weaver.uptodate
WEAVEEXEC_UPTODATE=.weaveexec.uptodate
PLUGIN_UPTODATE=.plugin.uptodate
WEAVEKUBE_UPTODATE=.weavekube.uptodate
WEAVEDB_UPTODATE=.weavedb.uptodate

IMAGES_UPTODATE=$(WEAVER_UPTODATE) $(WEAVEEXEC_UPTODATE) $(PLUGIN_UPTODATE) $(WEAVEKUBE_UPTODATE) $(WEAVEDB_UPTODATE)

WEAVER_IMAGE=$(DOCKERHUB_USER)/weave
WEAVEEXEC_IMAGE=$(DOCKERHUB_USER)/weaveexec
PLUGIN_IMAGE=$(DOCKERHUB_USER)/plugin
WEAVEKUBE_IMAGE=$(DOCKERHUB_USER)/weave-kube
BUILD_IMAGE=$(DOCKERHUB_USER)/weavebuild
WEAVEDB_IMAGE=$(DOCKERHUB_USER)/weavedb

IMAGES=$(WEAVER_IMAGE) $(WEAVEEXEC_IMAGE) $(PLUGIN_IMAGE) $(WEAVEKUBE_IMAGE) $(WEAVEDB_IMAGE)

WEAVE_EXPORT=weave.tar.gz

WEAVEEXEC_DOCKER_VERSION=1.6.2
DOCKER_DISTRIB=prog/weaveexec/docker-$(WEAVEEXEC_DOCKER_VERSION).tgz
DOCKER_DISTRIB_URL=https://get.docker.com/builds/Linux/x86_64/docker-$(WEAVEEXEC_DOCKER_VERSION).tgz
NETGO_CHECK=@strings $@ | grep cgo_stub\\\.go >/dev/null || { \
	rm $@; \
	echo "\nYour go standard library was built without the 'netgo' build tag."; \
	echo "To fix that, run"; \
	echo "    sudo go clean -i net"; \
	echo "    sudo go install -tags netgo std"; \
	false; \
}
BUILD_FLAGS=-i -ldflags "-linkmode external -extldflags -static -X main.version=$(WEAVE_VERSION)" -tags netgo

PACKAGE_BASE=$(shell go list -e ./)

all: $(WEAVE_EXPORT)
testrunner: $(RUNNER_EXE) $(TEST_TLS_EXE)

$(WEAVER_EXE) $(WEAVEPROXY_EXE) $(WEAVEUTIL_EXE): common/*.go common/*/*.go net/*.go net/*/*.go
$(WEAVER_EXE): router/*.go ipam/*.go ipam/*/*.go db/*.go nameserver/*.go prog/weaver/*.go
$(WEAVEPROXY_EXE): proxy/*.go prog/weaveproxy/*.go
$(WEAVEUTIL_EXE): prog/weaveutil/*.go net/*.go
$(SIGPROXY_EXE): prog/sigproxy/*.go
$(KUBEPEERS_EXE): prog/kube-peers/*.go
$(PLUGIN_EXE): prog/plugin/*.go plugin/*/*.go api/*.go common/*.go common/docker/*.go net/*.go
$(TEST_TLS_EXE): test/tls/*.go
$(WEAVEWAIT_NOOP_EXE): prog/weavewait/*.go
$(WEAVEWAIT_EXE): prog/weavewait/*.go net/*.go
$(WEAVEWAIT_NOMCAST_EXE): prog/weavewait/*.go net/*.go
tests: tools/.git
lint: tools/.git

ifeq ($(BUILD_IN_CONTAINER),true)

exes $(EXES) tests lint: $(BUILD_UPTODATE)
	git submodule update --init
	@mkdir -p $(shell pwd)/.pkg
	$(SUDO) docker run $(RM) $(RUN_FLAGS) \
	    -v $(shell pwd):/go/src/github.com/weaveworks/weave \
		-v $(shell pwd)/.pkg:/go/pkg \
		-e GOARCH -e GOOS -e CIRCLECI -e CIRCLE_BUILD_NUM -e CIRCLE_NODE_TOTAL -e CIRCLE_NODE_INDEX -e COVERDIR -e SLOW \
		$(BUILD_IMAGE) COVERAGE=$(COVERAGE) WEAVE_VERSION=$(WEAVE_VERSION) $@

else

exes: $(EXES)

$(WEAVER_EXE) $(WEAVEPROXY_EXE) $(PLUGIN_EXE):
ifeq ($(COVERAGE),true)
	$(eval COVERAGE_MODULES := $(shell (go list ./$(@D); go list -f '{{join .Deps "\n"}}' ./$(@D) | grep "^$(PACKAGE_BASE)/") | grep -v "^$(PACKAGE_BASE)/vendor/" | paste -s -d,))
	go test -c -o ./$@ $(BUILD_FLAGS) -v -covermode=atomic -coverpkg $(COVERAGE_MODULES) ./$(@D)/
else
	go build $(BUILD_FLAGS) -o $@ ./$(@D)
endif
	$(NETGO_CHECK)

$(WEAVEUTIL_EXE) $(KUBEPEERS_EXE):
	go build $(BUILD_FLAGS) -o $@ ./$(@D)
	$(NETGO_CHECK)

$(WEAVEWAIT_EXE):
	go build $(BUILD_FLAGS) -tags "netgo iface mcast" -o $@ ./$(@D)
	$(NETGO_CHECK)

$(WEAVEWAIT_NOMCAST_EXE):
	go build $(BUILD_FLAGS) -tags "netgo iface" -o $@ ./$(@D)
	$(NETGO_CHECK)

# These programs need a separate rule as they fail the netgo check in
# the main build stanza due to not importing net package
$(SIGPROXY_EXE) $(TEST_TLS_EXE) $(WEAVEWAIT_NOOP_EXE):
	go build $(BUILD_FLAGS) -o $@ ./$(@D)

tests:
	./tools/test -no-go-get

lint:
	./tools/lint -nocomment -notestpackage .

endif

$(BUILD_UPTODATE): build/*
	$(SUDO) docker build -t $(BUILD_IMAGE) build/
	touch $@

%/Dockerfile.$(DOCKERHUB_USER): %/Dockerfile.template
	sed -e "s/DOCKERHUB_USER/$(DOCKERHUB_USER)/" $^ > $@

$(WEAVER_UPTODATE): prog/weaver/Dockerfile.$(DOCKERHUB_USER) $(WEAVER_EXE) $(WEAVEEXEC_UPTODATE)
	$(SUDO) DOCKER_HOST=$(DOCKER_HOST) docker build -f prog/weaver/Dockerfile.$(DOCKERHUB_USER) -t $(WEAVER_IMAGE) prog/weaver
	touch $@

$(WEAVEEXEC_UPTODATE): prog/weaveexec/Dockerfile prog/weaveexec/symlink $(DOCKER_DISTRIB) weave $(SIGPROXY_EXE) $(WEAVEPROXY_EXE) $(WEAVEWAIT_EXE) $(WEAVEWAIT_NOOP_EXE) $(WEAVEWAIT_NOMCAST_EXE) $(WEAVEUTIL_EXE)
	cp weave prog/weaveexec/weave
	cp $(SIGPROXY_EXE) prog/weaveexec/sigproxy
	cp $(WEAVEPROXY_EXE) prog/weaveexec/weaveproxy
	cp $(WEAVEWAIT_EXE) prog/weaveexec/weavewait
	cp $(WEAVEWAIT_NOOP_EXE) prog/weaveexec/weavewait_noop
	cp $(WEAVEWAIT_NOMCAST_EXE) prog/weaveexec/weavewait_nomcast
	cp $(WEAVEUTIL_EXE) prog/weaveexec/weaveutil
	cp $(DOCKER_DISTRIB) prog/weaveexec/docker.tgz
	$(SUDO) DOCKER_HOST=$(DOCKER_HOST) docker build -t $(WEAVEEXEC_IMAGE) prog/weaveexec
	touch $@

$(PLUGIN_UPTODATE): prog/plugin/Dockerfile.$(DOCKERHUB_USER) $(PLUGIN_EXE) $(WEAVER_UPTODATE)
	$(SUDO) docker build -f prog/plugin/Dockerfile.$(DOCKERHUB_USER) -t $(PLUGIN_IMAGE) prog/plugin
	touch $@

$(WEAVEKUBE_UPTODATE): prog/weave-kube/Dockerfile.$(DOCKERHUB_USER) $(KUBEPEERS_EXE) $(PLUGIN_UPTODATE)
	cp $(KUBEPEERS_EXE) prog/weave-kube/
	$(SUDO) docker build -f prog/weave-kube/Dockerfile.$(DOCKERHUB_USER) -t $(WEAVEKUBE_IMAGE) prog/weave-kube
	touch $@

$(WEAVEDB_UPTODATE): prog/weavedb/Dockerfile
	$(SUDO) docker build -t $(WEAVEDB_IMAGE) prog/weavedb
	touch $@

$(WEAVE_EXPORT): $(IMAGES_UPTODATE)
	$(SUDO) DOCKER_HOST=$(DOCKER_HOST) docker save $(addsuffix :latest,$(IMAGES)) | gzip > $@

$(DOCKER_DISTRIB):
	curl -o $(DOCKER_DISTRIB) $(DOCKER_DISTRIB_URL)

tools/.git:
	git submodule update --init

$(RUNNER_EXE): tools/.git
	GO15VENDOREXPERIMENT=1 make -C tools/runner

$(PUBLISH): publish_%: $(IMAGES_UPTODATE)
	$(SUDO) DOCKER_HOST=$(DOCKER_HOST) docker tag  $(DOCKERHUB_USER)/$* $(DOCKERHUB_USER)/$*:$(WEAVE_VERSION)
	$(SUDO) DOCKER_HOST=$(DOCKER_HOST) docker push $(DOCKERHUB_USER)/$*:$(WEAVE_VERSION)
ifneq ($(UPDATE_LATEST),false)
	$(SUDO) DOCKER_HOST=$(DOCKER_HOST) docker push $(DOCKERHUB_USER)/$*:latest
endif

publish: $(PUBLISH)
ifeq ($(PUBLISH_WEAVEDB),true)
	$(SUDO) DOCKER_HOST=$(DOCKER_HOST) docker push   $(DOCKERHUB_USER)/weavedb:latest
endif

clean-bin:
	-$(SUDO) DOCKER_HOST=$(DOCKER_HOST) docker rmi $(IMAGES)
	rm -rf $(EXES) $(IMAGES_UPTODATE) $(WEAVE_EXPORT) .pkg

clean: clean-bin
	-$(SUDO) DOCKER_HOST=$(DOCKER_HOST) docker rmi $(BUILD_IMAGE)
	rm -rf test/tls/*.pem test/coverage.* test/coverage $(BUILD_UPTODATE)

build:
	$(SUDO) go clean -i net
	$(SUDO) go install -tags netgo std
	$(MAKE)

run-smoketests: all testrunner
	cd test && ./setup.sh && ./run_all.sh
