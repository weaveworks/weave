PUBLISH=publish_weave publish_weavedns publish_weaveexec

.DEFAULT: all
.PHONY: all update tests publish $(PUBLISH) clean prerequisites build travis run-smoketests

# If you can use docker without being root, you can do "make SUDO="
SUDO=sudo

DOCKERHUB_USER=weaveworks
WEAVE_VERSION=git-$(shell git rev-parse --short=12 HEAD)

WEAVER_EXE=prog/weaver/weaver
WEAVEDNS_EXE=prog/weavedns/weavedns
WEAVEDISCOVERY_EXE=prog/weavediscovery/weavediscovery
WEAVEPROXY_EXE=prog/weaveproxy/weaveproxy
SIGPROXY_EXE=prog/sigproxy/sigproxy
WEAVEWAIT_EXE=prog/weavewait/weavewait
NETCHECK_EXE=prog/netcheck/netcheck

EXES=$(WEAVER_EXE) $(WEAVEDNS_EXE) $(WEAVEDISCOVERY_EXE) $(SIGPROXY_EXE) $(WEAVEPROXY_EXE) $(WEAVEWAIT_EXE) $(NETCHECK_EXE)

WEAVER_UPTODATE=.weaver.uptodate
WEAVEDNS_UPTODATE=.weavedns.uptodate
WEAVEDISCOVERY_UPTODATE=.weavediscovery.uptodate
WEAVEEXEC_UPTODATE=.weaveexec.uptodate

IMAGES_UPTODATE=$(WEAVER_UPTODATE) $(WEAVEDNS_UPTODATE) $(WEAVEDISCOVERY_UPTODATE) $(WEAVEEXEC_UPTODATE)

WEAVER_IMAGE=$(DOCKERHUB_USER)/weave
WEAVEDNS_IMAGE=$(DOCKERHUB_USER)/weavedns
WEAVEDISCOVERY_IMAGE=$(DOCKERHUB_USER)/weavediscovery
WEAVEEXEC_IMAGE=$(DOCKERHUB_USER)/weaveexec

IMAGES=$(WEAVER_IMAGE) $(WEAVEDNS_IMAGE) $(WEAVEDISCOVERY_IMAGE) $(WEAVEEXEC_IMAGE)

WEAVE_EXPORT=weave.tar

WEAVEEXEC_DOCKER_VERSION=1.3.1
DOCKER_DISTRIB=prog/weaveexec/docker-$(WEAVEEXEC_DOCKER_VERSION).tgz
DOCKER_DISTRIB_URL=https://get.docker.com/builds/Linux/x86_64/docker-$(WEAVEEXEC_DOCKER_VERSION).tgz

all: $(WEAVE_EXPORT)

travis: $(EXES)

update:
	go get -u -f -v -tags -netgo $(addprefix ./,$(dir $(EXES)))

$(WEAVER_EXE) $(WEAVEDNS_EXE) $(WEAVEDISCOVERY_EXE) $(WEAVEPROXY_EXE) $(NETCHECK_EXE): common/*.go common/*/*.go net/*.go
	go get -tags netgo ./$(@D)
	go build -ldflags "-extldflags \"-static\" -X main.version $(WEAVE_VERSION)" -tags netgo -o $@ ./$(@D)
	@strings $@ | grep cgo_stub\\\.go >/dev/null || { \
		rm $@; \
		echo "\nYour go standard library was built without the 'netgo' build tag."; \
		echo "To fix that, run"; \
		echo "    sudo go clean -i net"; \
		echo "    sudo go install -tags netgo std"; \
		false; \
	}

$(WEAVER_EXE): router/*.go ipam/*.go ipam/*/*.go prog/weaver/main.go
$(WEAVEDNS_EXE): nameserver/*.go prog/weavedns/main.go
$(WEAVEDISCOVERY_EXE): prog/weavediscovery/*.go
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

$(WEAVEDISCOVERY_UPTODATE): prog/weavediscovery/Dockerfile $(WEAVEDISCOVERY_EXE)
	$(SUDO) docker build -t $(WEAVEDISCOVERY_IMAGE) prog/weavediscovery
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
	echo "mode: count" > profile.cov
	fail=0 ; \
	for dir in $$(find . -type f -name '*_test.go' | xargs -n1 dirname | sort -u); do \
		go get -t -tags netgo $$dir ; \
		output=$$(mktemp cover.XXXXXXXXXX) ; \
		if ! go test -cpu 4 -tags netgo \
			-covermode=count -coverprofile=$$output $$dir ; then \
		fail=1 ; \
        fi ; \
	if [ -f $$output ]; then \
		tail -n +2 <$$output >>profile.cov; \
		rm $$output; \
	fi \
	done ; \
	exit $$fail
	go tool cover -html=profile.cov -o=coverage.html

$(PUBLISH): publish_%:
	$(SUDO) docker tag -f $(DOCKERHUB_USER)/$* $(DOCKERHUB_USER)/$*:$(WEAVE_VERSION)
	$(SUDO) docker push   $(DOCKERHUB_USER)/$*:$(WEAVE_VERSION)
	$(SUDO) docker push   $(DOCKERHUB_USER)/$*:latest

publish: $(PUBLISH)

clean:
	-$(SUDO) docker rmi $(IMAGES)
	rm -f $(EXES) $(IMAGES_UPTODATE) $(WEAVE_EXPORT)
	rm -f test/tls/*.pem

build:
	$(SUDO) go clean -i net
	$(SUDO) go install -tags netgo std
	$(MAKE)

run-smoketests: all
	cd test && ./setup.sh && ./run_all.sh
