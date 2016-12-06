.DEFAULT: all
.PHONY: all clean image publish

DH_ORG=weaveworks
IMAGE_TAG=latest

all: image

clean:
	rm -rf ./build

build/.image.done: Dockerfile test-npc
	mkdir -p build
	cp $^ build
	sudo docker build -t $(DH_ORG)/test-npc:$(IMAGE_TAG) -f build/Dockerfile ./build
	touch $@

image: build/.image.done

publish: image
	sudo docker push $(DH_ORG)/test-npc:$(IMAGE_TAG)
