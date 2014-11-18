.DEFAULT: all
.PHONY: all publish clean

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

publish: $(WEAVER_EXPORT)
	/bin/bash scripts/set_version.sh
	$(SUDO) docker tag $(WEAVER_IMAGE) $(WEAVER_IMAGE):git-`git describe --tags`
	$(SUDO) docker push $(WEAVER_IMAGE):latest
	$(SUDO) docker push $(WEAVER_IMAGE):git-`git describe --tags`
	/usr/bin/git push origin master --tags
	$(SUDO) docker tag $(WEAVEDNS_IMAGE) $(WEAVEDNS_IMAGE):git-`git describe --tags` 
	$(SUDO) docker push $(WEAVEDNS_IMAGE):latest
	$(SUDO) docker push $(WEAVEDNS_IMAGE):git-`git describe --tags`

clean:
	-$(SUDO) docker rmi $(WEAVER_IMAGE) $(WEAVEDNS_IMAGE)
	rm -f $(WEAVER_EXE) $(WEAVEDNS_EXE) $(WEAVER_EXPORT) $(WEAVEDNS_EXPORT)

