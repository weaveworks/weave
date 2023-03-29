IMAGE_VERSION ?= 2.8.2-beta5
REGISTRY_USER ?= weaveworks

# Targets
.PHONY: build
build: submodule
	REGISTRY_USER=${REGISTRY_USER} IMAGE_VERSION=${IMAGE_VERSION} \
	PLATFORMS="$$(docker version -f '{{ .Server.Arch }}')" \
	POSTBUILD=--load \
	tools/build-images.sh

.PHONY: scan
scan:
	REGISTRY_USER=${REGISTRY_USER} IMAGE_VERSION=${IMAGE_VERSION} \
	tools/scan-images.sh

.PHONY: publish
publish: submodule
	REGISTRY_USER=${REGISTRY_USER} IMAGE_VERSION=${IMAGE_VERSION} \
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
