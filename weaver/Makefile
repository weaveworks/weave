.DEFAULT: all

all: docker-image

weaver: ../*.go main.go
	go build -ldflags '-extldflags "-static"'

.PHONY: docker-image

docker-image: /tmp/weave.tar

/tmp/weave.tar: Dockerfile weaver
	sudo docker build -t zettio/weave .
	sudo docker save zettio/weave > /tmp/weave.tar

publish: docker-image
	sudo docker tag zettio/weave zettio/weave:git-`git rev-parse --short=12 HEAD`
	sudo docker push zettio/weave

clean:
	-sudo docker rmi zettio/weave
	rm -f weaver /tmp/weave.tar
