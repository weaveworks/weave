.DEFAULT: all
.PHONY: all update tests publish $(PUBLISH) clean

# If you can use docker without being root, you can do "make SUDO="
SUDO=sudo

DOCKERHUB_USER=zettio
WEAVE_VERSION=git-$(shell git rev-parse --short=12 HEAD)
WEAVER_EXE=weaver/weaver
WEAVEDNS_EXE=weavedns/weavedns
WEAVETOOLS_EXES=tools/bin
WEAVEDISC_EXE=cmd/discovery/weavediscovery
WEAVER_IMAGE=$(DOCKERHUB_USER)/weave
WEAVEDNS_IMAGE=$(DOCKERHUB_USER)/weavedns
WEAVETOOLS_IMAGE=$(DOCKERHUB_USER)/weavetools
WEAVEDISC_IMAGE=$(DOCKERHUB_USER)/weavediscovery
WEAVER_EXPORT=/var/tmp/weave.tar
WEAVEDNS_EXPORT=/var/tmp/weavedns.tar
WEAVETOOLS_EXPORT=/var/tmp/weavetools.tar
WEAVEDISC_EXPORT=/var/tmp/weavediscovery.tar

all: $(WEAVER_EXPORT) $(WEAVEDNS_EXPORT) $(WEAVETOOLS_EXPORT) $(WEAVEDISC_EXPORT)

update:
	go get -u -f -v -tags -netgo ./$(dir $(WEAVER_EXE)) ./$(dir $(WEAVEDNS_EXE))

$(WEAVER_EXE) $(WEAVEDNS_EXE) $(WEAVEDISC_EXE):
	go get -tags netgo ./$(@D)
	go build -ldflags "-extldflags \"-static\" -X main.version $(WEAVE_VERSION)" -tags netgo -o $@ ./$(shell dirname $@)
	@strings $@ | grep cgo_stub\\\.go >/dev/null || { \
		rm $@; \
		echo "\nYour go standard library was built without the 'netgo' build tag."; \
		echo "To fix that, run"; \
		echo "    sudo go clean -i net"; \
		echo "    sudo go install -tags netgo std"; \
		false; \
	}

$(WEAVER_EXE):      common/*.go router/*.go weaver/main.go
$(WEAVEDNS_EXE):    common/*.go nameserver/*.go weavedns/main.go
$(WEAVEDISC_EXE): common/*.go nameserver/*.go discovery/*.go cmd/discovery/*.go

$(WEAVETOOLS_EXES): tools/build.sh
	$(SUDO) docker run --rm -v $(realpath $(<D)):/home/weave ubuntu sh /home/weave/build.sh

$(WEAVER_EXPORT): weaver/Dockerfile $(WEAVER_EXE)
	$(SUDO) docker build -t $(WEAVER_IMAGE) weaver
	$(SUDO) docker save $(WEAVER_IMAGE):latest > $@

$(WEAVEDNS_EXPORT): weavedns/Dockerfile $(WEAVEDNS_EXE)
	$(SUDO) docker build -t $(WEAVEDNS_IMAGE) weavedns
	$(SUDO) docker save $(WEAVEDNS_IMAGE):latest > $@

$(WEAVETOOLS_EXPORT): tools/Dockerfile $(WEAVETOOLS_EXES)
	$(SUDO) docker build -t $(WEAVETOOLS_IMAGE) tools
	$(SUDO) docker save $(WEAVETOOLS_IMAGE):latest > $@

$(WEAVEDISC_EXPORT): cmd/discovery/Dockerfile $(WEAVEDISC_EXE)
	$(SUDO) docker build -t $(WEAVEDISC_IMAGE) cmd/discovery
	$(SUDO) docker save $(WEAVEDISC_IMAGE):latest > $@

# Add more directories in here as more tests are created
tests:
	cd router; go test -tags netgo
	cd nameserver; go test -tags netgo

$(PUBLISH): publish_%:
	$(SUDO) docker tag -f $(DOCKERHUB_USER)/$* $(DOCKERHUB_USER)/$*:$(WEAVE_VERSION)
	$(SUDO) docker push   $(DOCKERHUB_USER)/$*:$(WEAVE_VERSION)
	$(SUDO) docker push   $(DOCKERHUB_USER)/$*:latest

publish: $(PUBLISH)

clean:
	-$(SUDO) docker rmi $(WEAVER_IMAGE) $(WEAVEDNS_IMAGE) $(WEAVETOOLS_IMAGE) $(WEAVEDISC_IMAGE)
	rm -f $(WEAVER_EXE) $(WEAVEDNS_EXE) $(WEAVER_EXPORT) $(WEAVEDNS_EXPORT) $(WEAVETOOLS_EXPORT) $(WEAVEDISC_EXPORT)
	$(SUDO) rm -rf $(WEAVETOOLS_EXES) $(WEAVEDISC_EXES)
