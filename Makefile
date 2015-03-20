PUBLISH=publish_weave publish_weavedns publish_weavetools

.DEFAULT: all
.PHONY: all update tests publish $(PUBLISH) clean prerequisites build

# If you can use docker without being root, you can do "make SUDO="
SUDO=sudo

DOCKERHUB_USER=zettio
WEAVE_VERSION=git-$(shell git rev-parse --short=12 HEAD)
WEAVER_EXE=weaver/weaver
WEAVEDNS_EXE=weavedns/weavedns
WEAVETOOLS_EXES=tools/bin
WEAVER_IMAGE=$(DOCKERHUB_USER)/weave
WEAVEDNS_IMAGE=$(DOCKERHUB_USER)/weavedns
WEAVETOOLS_IMAGE=$(DOCKERHUB_USER)/weavetools
WEAVER_EXPORT=weave.tar
WEAVEDNS_EXPORT=weavedns.tar
WEAVETOOLS_EXPORT=weavetools.tar

all: $(WEAVER_EXPORT) $(WEAVEDNS_EXPORT) $(WEAVETOOLS_EXPORT)

update:
	go get -u -f -v -tags -netgo ./$(dir $(WEAVER_EXE)) ./$(dir $(WEAVEDNS_EXE))

$(WEAVER_EXE) $(WEAVEDNS_EXE): common/*.go
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

$(WEAVER_EXE): router/*.go ipam/*.go weaver/main.go
$(WEAVEDNS_EXE): nameserver/*.go weavedns/main.go

build_weavetools_exes_in_container=yes

$(WEAVETOOLS_EXES): tools/build.sh
ifdef build_weavetools_exes_in_container
	$(SUDO) docker run --rm -t -v $(realpath $(<D)):/home/weave ubuntu sh /home/weave/build.sh
else
	rm -rf tools/build
	mkdir tools/build
	cd tools/build && sh ../../$<
	rm -rf tools/build
endif

$(WEAVER_EXPORT): weaver/Dockerfile $(WEAVER_EXE)
	$(SUDO) docker build -t $(WEAVER_IMAGE) weaver
	$(SUDO) docker save $(WEAVER_IMAGE):latest > $@

$(WEAVEDNS_EXPORT): weavedns/Dockerfile $(WEAVEDNS_EXE)
	$(SUDO) docker build -t $(WEAVEDNS_IMAGE) weavedns
	$(SUDO) docker save $(WEAVEDNS_IMAGE):latest > $@

$(WEAVETOOLS_EXPORT): tools/Dockerfile $(WEAVETOOLS_EXES)
	$(SUDO) docker build -t $(WEAVETOOLS_IMAGE) tools
	$(SUDO) docker save $(WEAVETOOLS_IMAGE):latest > $@

# Add more directories in here as more tests are created
tests:
	cd router; go test -cover -tags netgo
	cd ipam; go test -cover -tags netgo
	cd nameserver; go test -cover -tags netgo

$(PUBLISH): publish_%:
	$(SUDO) docker tag -f $(DOCKERHUB_USER)/$* $(DOCKERHUB_USER)/$*:$(WEAVE_VERSION)
	$(SUDO) docker push   $(DOCKERHUB_USER)/$*:$(WEAVE_VERSION)
	$(SUDO) docker push   $(DOCKERHUB_USER)/$*:latest

publish: $(PUBLISH)

clean:
	-$(SUDO) docker rmi $(WEAVER_IMAGE) $(WEAVEDNS_IMAGE) $(WEAVETOOLS_IMAGE)
	rm -f $(WEAVER_EXE) $(WEAVEDNS_EXE) $(WEAVER_EXPORT) $(WEAVEDNS_EXPORT) $(WEAVETOOLS_EXPORT)
	$(SUDO) rm -rf $(WEAVETOOLS_EXES)

build:
	$(SUDO) go clean -i net
	$(SUDO) go install -tags netgo std
	$(MAKE) build_weavetools_exes_in_container=
