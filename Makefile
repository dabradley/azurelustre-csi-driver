# Copyright 2017 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

PKG = sigs.k8s.io/azurelustre-csi-driver
GIT_COMMIT ?= $(shell git rev-parse HEAD)
REGISTRY ?= azurelustre.azurecr.io
IMAGE_NAME = azurelustre-csi
IMAGE_VERSION ?= latest
IMAGE_TAG ?= $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_VERSION)
LATEST_TAG ?= latest
IMAGE_TAG_LATEST = $(REGISTRY)/$(IMAGE_NAME):$(LATEST_TAG)
COMMIT_TAG ?= $(LATEST_TAG)-$(GIT_COMMIT)
IMAGE_TAG_COMMIT = $(REGISTRY)/$(IMAGE_NAME):$(COMMIT_TAG)
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS ?= "-X ${PKG}/pkg/azurelustre.driverVersion=${IMAGE_VERSION} -X ${PKG}/pkg/azurelustre.gitCommit=${GIT_COMMIT} -X ${PKG}/pkg/azurelustre.buildDate=${BUILD_DATE} -s -w -extldflags '-static'"
GINKGO_FLAGS = -ginkgo.v
GO111MODULE = on
GOPATH ?= $(shell go env GOPATH)
GOBIN ?= $(GOPATH)/bin
CGO_ENABLED ?= 1
export GOPATH GOBIN GO111MODULE CGO_ENABLED

ARCH ?= amd64
FLAVORS := jammy noble

all: azurelustre

#
# Tests
#
.PHONY: verify
verify: unit-test
	hack/verify-all.sh

.PHONY: unit-test
unit-test:
	go test -covermode=count -coverprofile=profile.cov ./pkg/... ./test/utils/credentials

.PHONY: sanity-test
sanity-test: azurelustre
	go test -v -timeout=30m ./test/sanity

.PHONY: sanity-test-local
sanity-test-local:
	go test -v -timeout=30m ./test/sanity_local -ginkgo.skip="should fail when requesting to create a volume with already existing name and different capacity|should fail when the requested volume does not exist"

.PHONY: e2e-test
e2e-test:
	if [ ! -z "$(EXTERNAL_E2E_TEST_AZURELUSTRE)" ]; then \
		bash ./test/external-e2e/run.sh;\
	else \
		go test -v -timeout=0 ./test/e2e ${GINKGO_FLAGS};\
	fi

#
# Azure Lustre: Code build
#
.PHONY: quicklustre
quicklustre:
	GOOS=linux GOARCH=$(ARCH) go build -ldflags ${LDFLAGS} -mod vendor -o _output/azurelustreplugin ./pkg/azurelustreplugin

.PHONY: azurelustre
azurelustre:
	GOOS=linux GOARCH=$(ARCH) go build -a -ldflags ${LDFLAGS} -mod vendor -o _output/azurelustreplugin ./pkg/azurelustreplugin

.PHONY: azurelustre-dalec
azurelustre-dalec:
	GOOS=linux go build -a -ldflags ${LDFLAGS} -mod vendor -o /app/azurelustreplugin ./pkg/azurelustreplugin

#
# Azure Lustre: Docker build
#
.PHONY: docker-build
docker-build:
	docker build --platform=linux/$(ARCH) -t $(IMAGE_TAG)-jammy --build-arg srcImage=ubuntu:22.04 --output=type=docker -f ./pkg/azurelustreplugin/Dockerfile .
	docker build --platform=linux/$(ARCH) -t $(IMAGE_TAG)-noble --build-arg srcImage=ubuntu:24.04 --output=type=docker -f ./pkg/azurelustreplugin/Dockerfile .
.PHONY: quickcontainer
quickcontainer: quicklustre docker-build
.PHONY: container
container: azurelustre docker-build

#
# Azure Lustre: Docker tag & push
#
.PHONY: push
push:
	$(foreach f,$(FLAVORS),docker push $(IMAGE_TAG)-$(f);)

.PHONY: tag-latest
tag-latest:
	$(foreach f,$(FLAVORS),docker tag $(IMAGE_TAG)-$(f) $(IMAGE_TAG_LATEST)-$(f);)

.PHONY: push-latest
push-latest: tag-latest
	$(foreach f,$(FLAVORS),docker push $(IMAGE_TAG_LATEST)-$(f);)

.PHONY: tag-commit
tag-commit: tag-latest
	$(foreach f,$(FLAVORS),docker tag $(IMAGE_TAG_LATEST)-$(f) $(IMAGE_TAG_COMMIT)-$(f);)

.PHONY: push-commit
push-commit: tag-commit
	$(foreach f,$(FLAVORS),docker push $(IMAGE_TAG_COMMIT)-$(f);)

.PHONY: build-push
build-push: container push

.PHONY: build-push-quick
build-push-quick: quickcontainer push

.PHONY: build-push-latest
build-push-latest: container push-latest

.PHONY: build-push-latest-commit
build-push-latest-commit: container push-latest push-commit

.PHONY: build-push-quick-latest
build-push-quick-latest: quickcontainer push-latest

.PHONY: clean
clean:
	go clean -r -x
	-rm -rf _output
