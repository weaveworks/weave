.DEFAULT: all

all: docker-image

weaver: ../*.go main.go
	go build -ldflags '-extldflags "-static"'

.PHONY: docker-image

docker-image: /tmp/weave.tar

/tmp/weave.tar: Dockerfile weaver
	sudo docker build -t zettio/weave .
	sudo docker save zettio/weave > /tmp/weave.tar

clean:
	-sudo docker rmi zettio/weave
	rm -f weaver /tmp/weave.tar
