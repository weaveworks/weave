.DEFAULT: all

all: server-image

weavedns: ../nameserver/*.go ./main.go
	go get -tags netgo && go build -tags netgo -ldflags '-linkmode external -extldflags "-static"' -o $@ -v

.PHONY: server-image

server-image: /var/tmp/weavedns.tar

/var/tmp/weavedns.tar: Dockerfile weavedns
	sudo -E docker build -t zettio/weavedns .
	sudo -E docker save zettio/weavedns > $@

clean:
	-sudo docker rmi zettio/weavedns
	rm -f weavedns
	rm -f /var/tmp/weavedns.tar
