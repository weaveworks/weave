IMAGE:=/tmp/weavedns.tar
MAIN:=../

.DEFAULT: docker-image

all: docker-image

weavedns: ../server/*.go ../main.go
	go get $(MAIN) && GOARCH=amd64 GOOS=linux go build -ldflags '-linkmode external -extldflags "-static"' -o $@ -v $(MAIN)

.PHONY: docker-image

docker-image: $(IMAGE)

$(IMAGE): Dockerfile weavedns
	sudo -E docker build -t zettio/weavedns .
	sudo -E docker save zettio/weavedns > $(IMAGE)

clean:
	-sudo docker rmi zettio/weavedns
	rm -f weavedns $(IMAGE)
