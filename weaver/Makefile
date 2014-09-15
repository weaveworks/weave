.DEFAULT: all

all: docker-image

weaver: ../*.go main.go
	go build -ldflags '-extldflags "-static"'

.PHONY: docker-image

docker-image: /tmp/weave.tar

# override this on the command-line when you are doing a release
# e.g. make tag=0.99
tag=dev

/tmp/weave.tar: Dockerfile weaver
	sudo docker build -t zettio/weave:latest .
	sudo docker tag zettio/weave:latest zettio/weave:$(tag)
	sudo docker save zettio/weave > /tmp/weave.tar

clean:
	-sudo docker rmi zettio/weave
	rm -f weaver pipework /tmp/weave.tar
