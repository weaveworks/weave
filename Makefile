.DEFAULT: all
.PHONY: all publish clean

# If you can use docker without being root, you can do "make SUDO="
SUDO=sudo

WEAVER_EXE=weaver/weaver
WEAVER_IMAGE=zettio/weave
WEAVER_EXPORT=/var/tmp/weave.tar

all: $(WEAVER_EXPORT)

$(WEAVER_EXE): router/*.go weaver/main.go
	go get -tags netgo ./weaver
	go build -ldflags '-extldflags "-static"' -tags netgo -o $@ ./weaver
	@strings $@ | grep cgo_stub\\\.go >/dev/null || { \
		rm $@; \
		echo "\nYour go standard library was built without the 'netgo' build tag."; \
		echo "To fix that, run"; \
		echo "    sudo go install -a -tags netgo std"; \
		false; \
	}

$(WEAVER_EXPORT): weaver/Dockerfile $(WEAVER_EXE)
	$(SUDO) docker build -t $(WEAVER_IMAGE) weaver
	$(SUDO) docker save $(WEAVER_IMAGE):latest > $@

publish: $(WEAVER_EXPORT)
	/bin/bash scripts/set_version.sh
	$(SUDO) docker tag $(WEAVER_IMAGE) $(WEAVER_IMAGE):git-`git describe --tags`
	$(SUDO) docker push $(WEAVER_IMAGE):latest
	$(SUDO) docker push $(WEAVER_IMAGE):git-`git describe --tags`
	/usr/bin/git push origin master --tags

clean:
	-$(SUDO) docker rmi $(WEAVER_IMAGE)
	rm -f $(WEAVER_EXE) $(WEAVER_EXPORT)

