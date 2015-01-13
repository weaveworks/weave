.DEFAULT: all
.PHONY: all update tests publish $(PUBLISH) clean

# If you can use docker without being root, you can do "make SUDO="
SUDO=sudo

DOCKERHUB_USER=zettio
WEAVE_VERSION=git-$(shell git rev-parse --short=12 HEAD)
WEAVER_EXE=weaver/weaver
WEAVEDNS_EXE=weavedns/weavedns
WEAVETOOLS_EXES=tools/bin
WEAVERENDEZ_EXE=cmd/rendezvous/weaverendezvous
WEAVER_IMAGE=$(DOCKERHUB_USER)/weave
WEAVEDNS_IMAGE=$(DOCKERHUB_USER)/weavedns
WEAVETOOLS_IMAGE=$(DOCKERHUB_USER)/weavetools
WEAVERENDEZ_IMAGE=$(DOCKERHUB_USER)/weaverendezvous
WEAVER_EXPORT=/var/tmp/weave.tar
WEAVEDNS_EXPORT=/var/tmp/weavedns.tar
WEAVETOOLS_EXPORT=/var/tmp/weavetools.tar
WEAVERENDEZ_EXPORT=/var/tmp/weaverendezvous.tar

all: $(WEAVER_EXPORT) $(WEAVEDNS_EXPORT) $(WEAVETOOLS_EXPORT) $(WEAVERENDEZ_EXPORT)

update:
	go get -u -f -v -tags -netgo ./$(dir $(WEAVER_EXE)) ./$(dir $(WEAVEDNS_EXE))

$(WEAVER_EXE) $(WEAVEDNS_EXE) $(WEAVERENDEZ_EXE):
	go get -tags netgo ./$(@D)
	go build -ldflags "-extldflags \"-static\" -X main.version $(WEAVE_VERSION)" -tags netgo -o $@ ./$(shell dirname $@)
	@ldd $@ 2>/dev/null | grep "not a dynamic executable" >/dev/null || { \
		rm $@; \
		echo "\nYour go standard library was built without the 'netgo' build tag."; \
		echo "To fix that, run"; \
		echo "    sudo go clean -i net"; \
		echo "    sudo go install -tags netgo std"; \
		false; \
	}

$(WEAVER_EXE):      common/*.go router/*.go weaver/main.go
$(WEAVEDNS_EXE):    common/*.go nameserver/*.go weavedns/main.go
$(WEAVERENDEZ_EXE): common/*.go nameserver/*.go rendezvous/*.go cmd/rendezvous/*.go

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

$(WEAVERENDEZ_EXPORT): cmd/rendezvous/Dockerfile $(WEAVERENDEZ_EXE)
	$(SUDO) docker build -t $(WEAVERENDEZ_IMAGE) cmd/rendezvous
	$(SUDO) docker save $(WEAVERENDEZ_IMAGE):latest > $@

# Add more directories in here as more tests are created
tests:
	cd nameserver; go test -tags netgo

$(PUBLISH): publish_%:
	$(SUDO) docker tag  $(DOCKERHUB_USER)/$* $(DOCKERHUB_USER)/$*:$(WEAVE_VERSION)
	$(SUDO) docker push $(DOCKERHUB_USER)/$*:$(WEAVE_VERSION)
	$(SUDO) docker push $(DOCKERHUB_USER)/$*:latest

publish: $(PUBLISH)

clean:
	-$(SUDO) docker rmi $(WEAVER_IMAGE) $(WEAVEDNS_IMAGE) $(WEAVETOOLS_IMAGE) $(WEAVERENDEZ_IMAGE)
	rm -f $(WEAVER_EXE) $(WEAVEDNS_EXE) $(WEAVER_EXPORT) $(WEAVEDNS_EXPORT) $(WEAVETOOLS_EXPORT) $(WEAVERENDEZ_EXPORT)
	$(SUDO) rm -rf $(WEAVETOOLS_EXES) $(WEAVERENDEZ_EXES)
