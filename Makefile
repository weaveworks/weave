PUBLISH=publish_weave publish_weaveexec publish_plugin

.DEFAULT: all
.PHONY: all update tests lint publish $(PUBLISH) clean clean-bin prerequisites build travis run-smoketests

# If you can use docker without being root, you can do "make SUDO="
SUDO=sudo

DOCKERHUB_USER=weaveworks
WEAVE_VERSION=git-$(shell git rev-parse --short=12 HEAD)

WEAVER_EXE=prog/weaver/weaver
WEAVEPROXY_EXE=prog/weaveproxy/weaveproxy
SIGPROXY_EXE=prog/sigproxy/sigproxy
WEAVEWAIT_EXE=prog/weavewait/weavewait
WEAVEWAIT_NOOP_EXE=prog/weavewait/weavewait_noop
WEAVEWAIT_NOMCAST_EXE=prog/weavewait/weavewait_nomcast
WEAVEUTIL_EXE=prog/weaveutil/weaveutil
DOCKERPLUGIN_EXE=prog/plugin/plugin
RUNNER_EXE=tools/runner/runner
TEST_TLS_EXE=test/tls/tls

EXES=$(WEAVER_EXE) $(SIGPROXY_EXE) $(WEAVEPROXY_EXE) $(WEAVEWAIT_EXE) $(WEAVEWAIT_NOOP_EXE) $(WEAVEWAIT_NOMCAST_EXE) $(WEAVEUTIL_EXE) $(DOCKERPLUGIN_EXE) $(TEST_TLS_EXE)

WEAVER_UPTODATE=.weaver.uptodate
WEAVEEXEC_UPTODATE=.weaveexec.uptodate
DOCKERPLUGIN_UPTODATE=.dockerplugin.uptodate

IMAGES_UPTODATE=$(WEAVER_UPTODATE) $(WEAVEEXEC_UPTODATE) $(DOCKERPLUGIN_UPTODATE)

WEAVER_IMAGE=$(DOCKERHUB_USER)/weave
WEAVEEXEC_IMAGE=$(DOCKERHUB_USER)/weaveexec
DOCKERPLUGIN_IMAGE=$(DOCKERHUB_USER)/plugin

IMAGES=$(WEAVER_IMAGE) $(WEAVEEXEC_IMAGE) $(DOCKERPLUGIN_IMAGE)

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
BUILD_FLAGS=-ldflags "-extldflags \"-static\" -X main.version $(WEAVE_VERSION)" -tags netgo

PACKAGE_BASE=$(shell go list -e ./)

all: $(WEAVE_EXPORT) $(RUNNER_EXE) $(TEST_TLS_EXE)

travis: $(EXES)

update:
	go get -u -f -v -tags netgo $(addprefix ./,$(dir $(EXES)))

$(WEAVER_EXE) $(WEAVEPROXY_EXE) $(WEAVEUTIL_EXE): common/*.go common/*/*.go net/*.go net/*/*.go
$(WEAVER_EXE): router/*.go mesh/*.go ipam/*.go ipam/*/*.go nameserver/*.go prog/weaver/*.go
$(WEAVEPROXY_EXE): proxy/*.go prog/weaveproxy/*.go
$(WEAVEUTIL_EXE): prog/weaveutil/*.go
$(SIGPROXY_EXE): prog/sigproxy/*.go
$(DOCKERPLUGIN_EXE): prog/plugin/*.go plugin/*/*.go api/*.go common/docker/*.go
$(TEST_TLS_EXE): test/tls/*.go
$(WEAVEWAIT_NOOP_EXE): prog/weavewait/*.go
$(WEAVEWAIT_EXE): prog/weavewait/*.go net/*.go
$(WEAVEWAIT_NOMCAST_EXE): prog/weavewait/*.go net/*.go

$(WEAVER_EXE) $(WEAVEPROXY_EXE):
ifeq ($(COVERAGE),true)
	$(eval COVERAGE_MODULES := $(shell (go list ./$(@D); go list -f '{{join .Deps "\n"}}' ./$(@D) | grep "^$(PACKAGE_BASE)/") | paste -s -d,))
	go get -t -tags netgo ./$(@D)
	go test -c -o ./$@ $(BUILD_FLAGS) -v -covermode=atomic -coverpkg $(COVERAGE_MODULES) ./$(@D)/
else
	go get -tags netgo ./$(@D)
	go build $(BUILD_FLAGS) -o $@ ./$(@D)
endif
	$(NETGO_CHECK)

$(WEAVEUTIL_EXE):
	go get -tags netgo ./$(@D)
	go build $(BUILD_FLAGS) -o $@ ./$(@D)
	$(NETGO_CHECK)

$(WEAVEWAIT_EXE):
	go get -tags netgo ./$(@D)
	go build $(BUILD_FLAGS) -tags "netgo iface mcast" -o $@ ./$(@D)
	$(NETGO_CHECK)

$(WEAVEWAIT_NOMCAST_EXE):
	go get -tags netgo ./$(@D)
	go build $(BUILD_FLAGS) -tags "netgo iface" -o $@ ./$(@D)
	$(NETGO_CHECK)

# These programs need a separat rule as they fail the netgo check in
# the main build stanza due to not importing net package
$(SIGPROXY_EXE) $(DOCKERPLUGIN_EXE) $(TEST_TLS_EXE) $(WEAVEWAIT_NOOP_EXE):
	go get -tags netgo ./$(@D)
	go build $(BUILD_FLAGS) -o $@ ./$(@D)

$(WEAVER_UPTODATE): prog/weaver/Dockerfile $(WEAVER_EXE)
	$(SUDO) DOCKER_HOST=$(DOCKER_HOST) docker build -t $(WEAVER_IMAGE) prog/weaver
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

$(DOCKERPLUGIN_UPTODATE): prog/plugin/Dockerfile $(DOCKERPLUGIN_EXE)
	$(SUDO) docker build -t $(DOCKERPLUGIN_IMAGE) prog/plugin
	touch $@

$(WEAVE_EXPORT): $(IMAGES_UPTODATE)
	$(SUDO) DOCKER_HOST=$(DOCKER_HOST) docker save $(addsuffix :latest,$(IMAGES)) | gzip > $@

$(DOCKER_DISTRIB):
	curl -o $(DOCKER_DISTRIB) $(DOCKER_DISTRIB_URL)

tests: tools/.git
	tools/test

lint: tools/.git
	tools/lint -nocomment -notestpackage .

tools/.git:
	git submodule update --init

$(RUNNER_EXE): tools/.git
	make -C tools/runner

$(PUBLISH): publish_%: $(IMAGES_UPTODATE)
	$(SUDO) DOCKER_HOST=$(DOCKER_HOST) docker tag -f $(DOCKERHUB_USER)/$* $(DOCKERHUB_USER)/$*:$(WEAVE_VERSION)
	$(SUDO) DOCKER_HOST=$(DOCKER_HOST) docker push   $(DOCKERHUB_USER)/$*:$(WEAVE_VERSION)
ifneq ($(UPDATE_LATEST),false)
	$(SUDO) DOCKER_HOST=$(DOCKER_HOST) docker push   $(DOCKERHUB_USER)/$*:latest
endif

publish: $(PUBLISH)

clean-bin:
	-$(SUDO) DOCKER_HOST=$(DOCKER_HOST) docker rmi $(IMAGES)
	go clean -r $(addprefix ./,$(dir $(EXES)))
	rm -f $(EXES) $(IMAGES_UPTODATE) $(WEAVE_EXPORT)

clean: clean-bin
	rm -rf test/tls/*.pem test/coverage.* test/coverage

build:
	$(SUDO) go clean -i net
	$(SUDO) go install -tags netgo std
	$(MAKE)

run-smoketests: all
	cd test && ./setup.sh && ./run_all.sh
