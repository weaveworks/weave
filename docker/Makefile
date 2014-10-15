.DEFAULT: all

all: server-image updater-image

weavedns: ../server/*.go ../main.go
	go get -tags netgo ../ && go build -tags netgo -ldflags '-linkmode external -extldflags "-static"' -o $@ -v ../

updater: ../updater/main.go
	go get -tags netgo ../updater/ && go build -tags netgo -ldflags '-linkmode external -extldflags "-static"' -o $@ -v ../updater/

.PHONY: server-image updater-image

updater-image: /var/tmp/updater.tar
server-image: /var/tmp/weavedns.tar

/var/tmp/weavedns.tar: Dockerfile weavedns
	sudo -E docker build -t zettio/weavedns .
	sudo -E docker save zettio/weavedns > $@

/var/tmp/updater.tar: ../updater/Dockerfile updater
	sudo -E docker build -t zettio/updater ../updater/
	sudo -E docker save zettio/updater > $@

clean:
	-sudo docker rmi zettio/weavedns
	-sudo docker rmi zettio/updater
	rm -f weavedns updater
	rm -f /var/tmp/{updater,weavedns}.tar
