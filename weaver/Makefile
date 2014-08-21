.DEFAULT: all

all: docker-image

weaver: ../*.go main.go
	go build -ldflags '-extldflags "-static"'

.PHONY: docker-image

docker-image: /tmp/weave.tar

/tmp/weave.tar: Dockerfile weaver
	sudo docker build -t zettio/weave .
	sudo docker save zettio/weave > /tmp/weave.tar

pipework:
	wget --no-check-certificate -nv https://raw.github.com/jpetazzo/pipework/master/pipework
	chmod +x pipework

clean:
	-sudo docker rmi zettio/weave
	rm -f weaver pipework /tmp/weave.tar
