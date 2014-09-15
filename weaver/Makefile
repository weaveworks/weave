.DEFAULT: all

all: docker-image

weaver: ../*.go main.go
	go build -ldflags '-extldflags "-static"'

.PHONY: docker-image

docker-image: /tmp/weave.tar

/tmp/weave.tar: Dockerfile weaver
	sudo docker build -t zettio/weave .
	sudo docker save zettio/weave > /tmp/weave.tar

# override this on the command-line when you are doing a release
# e.g. make publish tag=0.99
tag=latest

publish: docker-image
	git tag -a $(tag) -m "Release $(tag)"
	sudo docker tag zettio/weave:latest zettio/weave:$(tag)
	sudo docker push zettio/weave

clean:
	-sudo docker rmi zettio/weave
	rm -f weaver pipework /tmp/weave.tar
