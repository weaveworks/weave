IMAGE:=/tmp/weavedns.tar
MAIN:=../main

.DEFAULT: docker-image

all: docker-image

dns: ../*.go ../main/main.go
	go get $(MAIN) && GOARCH=amd64 GOOS=linux go build -o weavedns -v $(MAIN)

.PHONY: docker-image

docker-image: $(IMAGE)

$(IMAGE): Dockerfile dns
	sudo -E docker build -t zettio/weavedns .
	sudo -E docker save zettio/weavedns > $(IMAGE)

clean:
	-sudo docker rmi zettio/weavedns
	rm -f weavedns $(IMAGE)
