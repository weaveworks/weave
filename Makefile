PUBLISH=publish_weave publish_weavedns publish_weaveexec

.DEFAULT: all
.PHONY: all update tests publish $(PUBLISH) clean prerequisites build travis run-smoketests

# If you can use docker without being root, you can do "make SUDO="
SUDO=sudo

DOCKERHUB_USER=weaveworks
WEAVE_VERSION=git-$(shell git rev-parse --short=12 HEAD)

WEAVER_EXE=prog/weaver/weaver
WEAVEDNS_EXE=prog/weavedns/weavedns
WEAVEPROXY_EXE=prog/weaveproxy/weaveproxy
SIGPROXY_EXE=prog/sigproxy/sigproxy
WEAVEWAIT_EXE=prog/weavewait/weavewait
NETCHECK_EXE=prog/netcheck/netcheck

EXES=$(WEAVER_EXE) $(WEAVEDNS_EXE) $(SIGPROXY_EXE) $(WEAVEPROXY_EXE) $(WEAVEWAIT_EXE) $(NETCHECK_EXE)

WEAVER_UPTODATE=.weaver.uptodate
WEAVEDNS_UPTODATE=.weavedns.uptodate
WEAVEEXEC_UPTODATE=.weaveexec.uptodate

IMAGES_UPTODATE=$(WEAVER_UPTODATE) $(WEAVEDNS_UPTODATE) $(WEAVEEXEC_UPTODATE)

WEAVER_IMAGE=$(DOCKERHUB_USER)/weave
WEAVEDNS_IMAGE=$(DOCKERHUB_USER)/weavedns
WEAVEEXEC_IMAGE=$(DOCKERHUB_USER)/weaveexec

IMAGES=$(WEAVER_IMAGE) $(WEAVEDNS_IMAGE) $(WEAVEEXEC_IMAGE)

WEAVE_EXPORT=weave.tar

WEAVEEXEC_DOCKER_VERSION=1.3.1
DOCKER_DISTRIB=prog/weaveexec/docker-$(WEAVEEXEC_DOCKER_VERSION).tgz
DOCKER_DISTRIB_URL=https://get.docker.com/builds/Linux/x86_64/docker-$(WEAVEEXEC_DOCKER_VERSION).tgz
COVERAGE_MODULES=$(shell go list -f '{{join .Deps "\n"}}' ./prog/weaver | grep "weaveworks" | paste -s -d,)
NETGO_CHECK=@strings $@ | grep cgo_stub\\\.go >/dev/null || { \
	rm $@; \
	echo "\nYour go standard library was built without the 'netgo' build tag."; \
	echo "To fix that, run"; \
	echo "    sudo go clean -i net"; \
	echo "    sudo go install -tags netgo std"; \
	false; \
}

all: $(WEAVE_EXPORT)

travis: $(EXES)

update:
	go get -u -f -v -tags -netgo $(addprefix ./,$(dir $(EXES)))

$(WEAVER_EXE): common/*.go common/*/*.go net/*.go
ifeq ($(COVERAGE),true)
	go get -t -tags netgo ./$(@D)
	go test -c -o ./$@ -ldflags "-extldflags \"-static\" -X main.version $(WEAVE_VERSION)" \
		-tags netgo -v -covermode=atomic -coverpkg $(COVERAGE_MODULES) ./$(@D)/
else
	go get -tags netgo ./$(@D)
	go build -ldflags "-extldflags \"-static\" -X main.version $(WEAVE_VERSION)" -tags netgo -o $@ ./$(@D)
endif
	$(NETGO_CHECK)

$(WEAVEDNS_EXE) $(WEAVEPROXY_EXE) $(NETCHECK_EXE): common/*.go common/*/*.go net/*.go
	go get -tags netgo ./$(@D)
	go build -ldflags "-extldflags \"-static\" -X main.version $(WEAVE_VERSION)" -tags netgo -o $@ ./$(@D)
	$(NETGO_CHECK)

$(WEAVER_EXE): router/*.go ipam/*.go ipam/*/*.go prog/weaver/main.go
$(WEAVEDNS_EXE): nameserver/*.go prog/weavedns/main.go
$(WEAVEPROXY_EXE): proxy/*.go prog/weaveproxy/main.go
$(NETCHECK_EXE): prog/netcheck/netcheck.go

# Sigproxy and weavewait need separate rules as they fail the netgo check in
# the main build stanza due to not importing net package

$(SIGPROXY_EXE): prog/sigproxy/main.go
	go build -o $@ ./$(@D)

$(WEAVEWAIT_EXE): prog/weavewait/main.go
	go build -o $@ ./$(@D)

$(WEAVER_UPTODATE): prog/weaver/Dockerfile $(WEAVER_EXE)
	$(SUDO) docker build -t $(WEAVER_IMAGE) prog/weaver
	touch $@

$(WEAVEDNS_UPTODATE): prog/weavedns/Dockerfile $(WEAVEDNS_EXE)
	$(SUDO) docker build -t $(WEAVEDNS_IMAGE) prog/weavedns
	touch $@

$(WEAVEEXEC_UPTODATE): prog/weaveexec/Dockerfile $(DOCKER_DISTRIB) weave $(SIGPROXY_EXE) $(WEAVEPROXY_EXE) $(WEAVEWAIT_EXE) $(NETCHECK_EXE)
	cp weave prog/weaveexec/weave
	cp $(SIGPROXY_EXE) prog/weaveexec/sigproxy
	cp $(WEAVEPROXY_EXE) prog/weaveexec/weaveproxy
	cp $(WEAVEWAIT_EXE) prog/weaveexec/weavewait
	cp $(NETCHECK_EXE) prog/weaveexec/netcheck
	cp $(DOCKER_DISTRIB) prog/weaveexec/docker.tgz
	$(SUDO) docker build -t $(WEAVEEXEC_IMAGE) prog/weaveexec
	touch $@

$(WEAVE_EXPORT): $(IMAGES_UPTODATE)
	$(SUDO) docker save $(addsuffix :latest,$(IMAGES)) > $@

$(DOCKER_DISTRIB):
	curl -o $(DOCKER_DISTRIB) $(DOCKER_DISTRIB_URL)

tests:
	@test/units.sh

$(PUBLISH): publish_%:
	$(SUDO) docker tag -f $(DOCKERHUB_USER)/$* $(DOCKERHUB_USER)/$*:$(WEAVE_VERSION)
	$(SUDO) docker push   $(DOCKERHUB_USER)/$*:$(WEAVE_VERSION)
	$(SUDO) docker push   $(DOCKERHUB_USER)/$*:latest

publish: $(PUBLISH)

clean:
	-$(SUDO) docker rmi $(IMAGES)
	go clean -r ./...
	rm -f $(EXES) $(IMAGES_UPTODATE) $(WEAVE_EXPORT)
	rm -f test/tls/*.pem

build:
	$(SUDO) go clean -i net
	$(SUDO) go install -tags netgo std
	$(MAKE)

run-smoketests: all
	cd test && ./setup.sh && ./run_all.sh
