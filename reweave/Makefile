IMAGE_VERSION ?= 2.8.1
REGISTRY_USER ?= weaveworks

# Targets
.PHONY: scan
scan:
	REGISTRY_USER=${REGISTRY_USER} IMAGE_VERSION=${IMAGE_VERSION} \
	tools/scan-images.sh

.PHONY: clean-scan
clean-scan:
	tools/clean-scans.sh

.PHONY: clean
clean: clean-scan
