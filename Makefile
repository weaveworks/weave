PUBLISH=publish_weave publish_weavedns publish_weaveexec

.DEFAULT: all
.PHONY: all update tests publish $(PUBLISH) clean prerequisites build travis run-smoketests

# If you can use docker without being root, you can do "make SUDO="
SUDO=sudo

DOCKERHUB_USER=weaveworks
WEAVE_VERSION=git-$(shell git rev-parse --short=12 HEAD)

WEAVER_EXE=weaver/weaver
WEAVEDNS_EXE=weavedns/weavedns
WEAVEPROXY_EXE=weaveproxy/weaveproxy
SIGPROXY_EXE=sigproxy/sigproxy
WEAVEWAIT_EXE=weavewait/weavewait

EXES=$(WEAVER_EXE) $(WEAVEDNS_EXE) $(SIGPROXY_EXE) $(WEAVEPROXY_EXE) $(WEAVEWAIT_EXE)

WEAVER_IMAGE=$(DOCKERHUB_USER)/weave
WEAVEDNS_IMAGE=$(DOCKERHUB_USER)/weavedns
WEAVEEXEC_IMAGE=$(DOCKERHUB_USER)/weaveexec

IMAGES=$(WEAVER_IMAGE) $(WEAVEDNS_IMAGE) $(WEAVEEXEC_IMAGE)

WEAVER_EXPORT=weave.tar
WEAVEDNS_EXPORT=weavedns.tar
WEAVEEXEC_EXPORT=weaveexec.tar

EXPORTS=$(WEAVER_EXPORT) $(WEAVEDNS_EXPORT) $(WEAVEEXEC_EXPORT)

WEAVEEXEC_DOCKER_VERSION=1.3.1
DOCKER_DISTRIB=weaveexec/docker-$(WEAVEEXEC_DOCKER_VERSION).tgz
DOCKER_DISTRIB_URL=https://get.docker.com/builds/Linux/x86_64/docker-$(WEAVEEXEC_DOCKER_VERSION).tgz

all: $(EXPORTS)

travis: $(EXES)

update: $(EXES)
	go get -u -f -v -tags -netgo $(addprefix ./,$(dir $(EXES)))

$(WEAVER_EXE) $(WEAVEDNS_EXE) $(WEAVEPROXY_EXE) $(WEAVEWAIT_EXE): common/*.go
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

$(WEAVER_EXE): router/*.go ipam/*.go ipam/*/*.go weaver/main.go
$(WEAVEDNS_EXE): nameserver/*.go weavedns/main.go
$(WEAVEPROXY_EXE): proxy/*.go weaveproxy/main.go
$(WEAVEWAIT_EXE): weavewait/*.go weavewait/main.go

# Sigproxy needs separate rule as it fails the netgo check in the main
# build stanza due to not importing net package
$(SIGPROXY_EXE): sigproxy/main.go
	go build -o $@ ./$(@D)

$(WEAVER_EXPORT): weaver/Dockerfile $(WEAVER_EXE)
	$(SUDO) docker build -t $(WEAVER_IMAGE) weaver
	$(SUDO) docker save $(WEAVER_IMAGE):latest > $@

$(WEAVEDNS_EXPORT): weavedns/Dockerfile $(WEAVEDNS_EXE)
	$(SUDO) docker build -t $(WEAVEDNS_IMAGE) weavedns
	$(SUDO) docker save $(WEAVEDNS_IMAGE):latest > $@

$(WEAVEEXEC_EXPORT): weaveexec/Dockerfile $(DOCKER_DISTRIB) weave $(SIGPROXY_EXE) $(WEAVEPROXY_EXE) $(WEAVEWAIT_EXE)
	cp weave weaveexec/weave
	cp $(SIGPROXY_EXE) weaveexec/sigproxy
	cp $(WEAVEPROXY_EXE) weaveexec/weaveproxy
	cp $(WEAVEWAIT_EXE) weaveexec/weavewait
	cp $(DOCKER_DISTRIB) weaveexec/docker.tgz
	$(SUDO) docker build -t $(WEAVEEXEC_IMAGE) weaveexec
	$(SUDO) docker save $(WEAVEEXEC_IMAGE):latest > $@

$(DOCKER_DISTRIB):
	curl -o $(DOCKER_DISTRIB) $(DOCKER_DISTRIB_URL)

tests:
	echo "mode: count" > profile.cov
	fail=0 ;                                                                              \
	for dir in $$(find . -type f -name '*_test.go' | xargs -n1 dirname | sort -u); do     \
	    output=$$(mktemp cover.XXXXXXXXXX) ;                                              \
	    if ! go test -tags netgo -covermode=count -coverprofile=$$output $$dir ; then     \
            fail=1 ;                                                                          \
        fi ;                                                                                  \
	    if [ -f $$output ]; then                                                          \
	        tail -n +2 <$$output >>profile.cov;                                           \
	        rm $$output;                                                                  \
	    fi                                                                                \
	done ;                                                                                \
	exit $$fail
	go tool cover -html=profile.cov -o=coverage.html

test-integration:
	$(SUDO) cp $(CURDIR)/weave /usr/local/bin/weave
	cd integration-cli; go test -cover

$(PUBLISH): publish_%:
	$(SUDO) docker tag -f $(DOCKERHUB_USER)/$* $(DOCKERHUB_USER)/$*:$(WEAVE_VERSION)
	$(SUDO) docker push   $(DOCKERHUB_USER)/$*:$(WEAVE_VERSION)
	$(SUDO) docker push   $(DOCKERHUB_USER)/$*:latest

publish: $(PUBLISH)

clean:
	-$(SUDO) docker rmi $(IMAGES)
	rm -f $(EXES) $(EXPORTS)

build:
	$(SUDO) go clean -i net
	$(SUDO) go install -tags netgo std
	$(MAKE)

run-smoketests: all
	cd test && ./setup.sh && ./run_all.sh
