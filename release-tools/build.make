# Copyright (c) 2021 Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.
#

.PHONY: build-% build container-% container push-% push clean

# Choose podman or docker
ifeq (, $(shell which podman))
CONTAINER_BUILD=docker
else
CONTAINER_BUILD=podman
endif

# Registry used on push
REGISTRY_NAME=

# Revision
REV=$(shell git describe --long --tags --match='v*' --dirty 2>/dev/null || git rev-list -n1 HEAD)

# A "zfssa-xxx" image gets built if the current branch is "zfssa-xxx".
IMAGE_TAGS=$(shell git branch | grep '* zfssa-' | grep -v -e ' -> ' | sed -e 's/\* //')

# Images are named after the command contained in them.
IMAGE_NAME=$(REGISTRY_NAME)/$*

build-%:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-X main.version=$(REV) -extldflags "-static"' -o ./bin/$* ./cmd/$*

container-%: build-%
	$(CONTAINER_BUILD) build -t $*:latest -f $(shell if [ -e ./cmd/zfssa-csi-driver/$*/Dockerfile ]; then echo ./cmd/zfssa-csi-driver/$*/Dockerfile; else echo Dockerfile; fi) --label revision=$(REV) . --build-arg var_proxy=$(CONTAINER_PROXY)

push-%: container-%
	set -ex; \
	push_image () { \
		$(CONTAINER_BUILD) tag $*:latest $(IMAGE_NAME):$$tag; \
		$(CONTAINER_BUILD) push $(IMAGE_NAME):$$tag; \
	}; \
	for tag in $(IMAGE_TAGS); do \
		if [ "$$tag" = "canary" ] || echo "$$tag" | grep -q -e '-canary$$'; then \
			: "creating or overwriting canary image"; \
			push_image; \
		elif $(CONTAINER_BUILD) pull $(IMAGE_NAME):$$tag 2>&1 | tee /dev/stderr | grep -q "manifest for $(IMAGE_NAME):$$tag not found"; then \
			: "creating release image"; \
			push_image; \
		else \
			: "release image $(IMAGE_NAME):$$tag already exists, skipping push"; \
		fi; \
	done

build: $(CMDS:%=build-%)
container: $(CMDS:%=container-%)
push: $(CMDS:%=push-%)

clean:
	-rm -rf bin

