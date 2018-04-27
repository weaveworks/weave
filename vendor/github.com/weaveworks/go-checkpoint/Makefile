.PHONY: all build test lint

BUILD_IN_CONTAINER ?= true
RM=--rm
BUILD_UPTODATE=backend/.image.uptodate
BUILD_IMAGE=checkpoint_build

all: build

$(BUILD_UPTODATE): backend/*
	docker build -t $(BUILD_IMAGE) backend
	touch $@

ifeq ($(BUILD_IN_CONTAINER),true)

build test lint: $(BUILD_UPTODATE)
	$(SUDO) docker run $(RM) -ti \
		-v $(shell pwd):/go/src/github.com/weaveworks/go-checkpoint \
		-e GOARCH -e GOOS -e BUILD_IN_CONTAINER=false \
		$(BUILD_IMAGE) $@

else

build:
	go get .
	go build .

test:
	go get -t .
	go test

lint:
	./tools/lint -notestpackage .
	
endif

