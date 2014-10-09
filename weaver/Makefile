.DEFAULT: all

# If you can use docker without being root, you can do "make SUDO="
SUDO=sudo

all: docker-image

weaver: ../*.go main.go
	go get -tags netgo
	go build -ldflags '-extldflags "-static"' -tags netgo

.PHONY: docker-image
docker-image: /var/tmp/weave.tar

/var/tmp/weave.tar: Dockerfile weaver
	$(SUDO) docker build -t zettio/weave .
	$(SUDO) docker save zettio/weave > /var/tmp/weave.tar

publish: docker-image
	$(SUDO) docker tag zettio/weave zettio/weave:git-`git rev-parse --short=12 HEAD`
	$(SUDO) docker push zettio/weave:latest
	$(SUDO) docker push zettio/weave:git-`git rev-parse --short=12 HEAD`

clean:
	-$(SUDO) docker rmi zettio/weave
	rm -f weaver /var/tmp/weave.tar
