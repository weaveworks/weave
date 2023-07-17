IMAGE_VERSION ?= 2.8.2-beta7
REGISTRY_USER ?= weaveworks

ALPINE_BASEIMAGE ?= alpine:3.18.2

# Targets
.PHONY: build
build: submodule
	REGISTRY_USER=${REGISTRY_USER} IMAGE_VERSION=${IMAGE_VERSION} \
	PLATFORMS="$$(docker version -f '{{ .Server.Arch }}')" \
	ALPINE_BASEIMAGE=$(ALPINE_BASEIMAGE) \
	POSTBUILD=--load \
	tools/build-images.sh

.PHONY: scan
scan:
	REGISTRY_USER=${REGISTRY_USER} IMAGE_VERSION=${IMAGE_VERSION} \
	tools/scan-images.sh

.PHONY: publish
publish: submodule
	REGISTRY_USER=${REGISTRY_USER} IMAGE_VERSION=${IMAGE_VERSION} \
	ALPINE_BASEIMAGE=$(ALPINE_BASEIMAGE) \
	POSTBUILD=--push tools/build-images.sh

.PHONY: clean-images
clean-images:
	REGISTRY_USER=${REGISTRY_USER} IMAGE_VERSION=${IMAGE_VERSION} \
	tools/clean-images.sh

.PHONY: clean-scan
clean-scan:
	tools/clean-scans.sh

.PHONY: clean
clean: clean-scan clean-images

.PHONY: submodule
submodule: ../tools/.git

../tools/.git:
	git submodule update --init
