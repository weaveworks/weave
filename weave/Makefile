.DEFAULT: all

all: docker-image

weave: ../*.go main.go
	go build -ldflags '-extldflags "-static"'

.PHONY: docker-image

docker-image: /tmp/wedo.tar

/tmp/wedo.tar: Dockerfile weave
	sudo docker build -t zettio/weave .
	sudo docker save zettio/weave > /tmp/wedo.tar

pipework:
	wget --no-check-certificate -nv https://raw.github.com/jpetazzo/pipework/master/pipework
	chmod +x pipework

clean:
	-sudo docker rmi zettio/weave
	rm -f weave pipework /tmp/wedo.tar
