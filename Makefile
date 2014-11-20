.DEFAULT: all
.PHONY: all publish clean tests

# If you can use docker without being root, you can do "make SUDO="
SUDO=sudo

WEAVER_EXE=weaver/weaver
WEAVEDNS_EXE=weavedns/weavedns
WEAVER_IMAGE=zettio/weave
WEAVEDNS_IMAGE=zettio/weavedns
WEAVER_EXPORT=/var/tmp/weave.tar
WEAVEDNS_EXPORT=/var/tmp/weavedns.tar

all: $(WEAVER_EXPORT) $(WEAVEDNS_EXPORT)

$(WEAVER_EXE) $(WEAVEDNS_EXE):
	go get -tags netgo ./$(shell dirname $@)
	go build -ldflags '-extldflags "-static"' -tags netgo -o $@ ./$(shell dirname $@)
	@strings $@ | grep cgo_stub\\\.go >/dev/null || { \
		rm $@; \
		echo "\nYour go standard library was built without the 'netgo' build tag."; \
		echo "To fix that, run"; \
		echo "    sudo go install -a -tags netgo std"; \
		false; \
	}

$(WEAVER_EXE): router/*.go weaver/main.go
$(WEAVEDNS_EXE): nameserver/*.go weavedns/main.go

$(WEAVER_EXPORT): weaver/Dockerfile $(WEAVER_EXE)
	$(SUDO) docker build -t $(WEAVER_IMAGE) weaver
	$(SUDO) docker save $(WEAVER_IMAGE):latest > $@

$(WEAVEDNS_EXPORT): weavedns/Dockerfile $(WEAVEDNS_EXE)
	$(SUDO) docker build -t $(WEAVEDNS_IMAGE) weavedns
	$(SUDO) docker save $(WEAVEDNS_IMAGE):latest > $@

# Add more directories in here as more tests are created
tests:
	cd nameserver; go test

publish: $(WEAVER_EXPORT) tests
	$(SUDO) docker tag $(WEAVER_IMAGE) $(WEAVER_IMAGE):git-`git rev-parse --short=12 HEAD`
	$(SUDO) docker push $(WEAVER_IMAGE):latest
	$(SUDO) docker push $(WEAVER_IMAGE):git-`git rev-parse --short=12 HEAD`
	$(SUDO) docker tag $(WEAVEDNS_IMAGE) $(WEAVEDNS_IMAGE):git-`git rev-parse --short=12 HEAD`
	$(SUDO) docker push $(WEAVEDNS_IMAGE):latest
	$(SUDO) docker push $(WEAVEDNS_IMAGE):git-`git rev-parse --short=12 HEAD`

clean:
	-$(SUDO) docker rmi $(WEAVER_IMAGE) $(WEAVEDNS_IMAGE)
	rm -f $(WEAVER_EXE) $(WEAVEDNS_EXE) $(WEAVER_EXPORT) $(WEAVEDNS_EXPORT)
