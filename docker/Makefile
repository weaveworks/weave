IMAGE:=/tmp/weavedns.tar

.DEFAULT: docker-image

all: docker-image

dns: ../*.go main.go
	go get && GOARCH=amd64 GOOS=linux go build -v

.PHONY: docker-image

docker-image: $(IMAGE)

$(IMAGE): Dockerfile dns
	sudo -E docker build --rm -t zettio/weavedns .
	sudo -E docker save zettio/weavedns > $(IMAGE)

clean:
	-sudo docker rmi zettio/weavedns
	rm -f dns $(IMAGE)
