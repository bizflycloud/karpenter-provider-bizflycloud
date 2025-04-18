# Image URL to use all building/pushing image targets
IMG ?= cr-hn-1.bizflycloud.vn/31ff9581861a4d0ea4df5e7dda0f665d/karpenter-provider-bizflycloud:latest
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.28.0

# Obtain the version using git describe, defaulting to 'dev' if git is not available
VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo "dev")

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

# LDFLAGS for the build
LDFLAGS = -ldflags "-X main.version=$(VERSION) -X main.buildDate=$(shell date -u +'%Y-%m-%dT%H:%M:%SZ')"

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: lint
lint: ## Run golangci-lint on the code.
	golangci-lint run ./...

.PHONY: test
test: fmt vet ## Run tests.
	go test ./... -v -coverprofile=coverage.out

.PHONY: test-short
test-short: fmt vet ## Run tests with -short flag to skip long-running tests.
	go test ./... -v -short

.PHONY: test-coverage
test-coverage: test ## Display test coverage.
	go tool cover -html=coverage.out

.PHONY: manifests
manifests: ## Generate CRDs and RBAC.
	@echo "Validating CRDs..."
	@kubectl validate --schema $(shell kubectl api-resources --api-group=apiextensions.k8s.io -o name | grep customresourcedefinition) -f config/crd/bizflycloud.karpenter.sh_providerconfigs.yaml

##@ Build

.PHONY: build
build: fmt vet ## Build manager binary.
	go build $(LDFLAGS) -o bin/karpenter-provider-bizflycloud cmd/karpenter-provider-bizflycloud/main.go

.PHONY: run
run: fmt vet ## Run from your host.
	go run $(LDFLAGS) ./cmd/karpenter-provider-bizflycloud/main.go

.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	docker build --build-arg VERSION=$(VERSION) -t ${IMG} .

.PHONY: docker-build-dev
docker-build-dev: ## Build docker image with a dev tag.
	docker build --build-arg VERSION=$(VERSION) -t ${IMG} -t cr-hn-1.bizflycloud.vn/31ff9581861a4d0ea4df5e7dda0f665d/karpenter-provider-bizflycloud:dev .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	docker push ${IMG}

##@ Deployment

.PHONY: install-crds
install-crds: ## Install CRDs into a cluster.
	kubectl apply -f config/crd/

.PHONY: uninstall-crds
uninstall-crds: ## Uninstall CRDs from a cluster.
	kubectl delete -f config/crd/

.PHONY: deploy
deploy: install-crds ## Deploy to the K8s cluster specified in ~/.kube/config.
	kubectl apply -f config/

.PHONY: undeploy
undeploy: ## Undeploy from the K8s cluster specified in ~/.kube/config.
	kubectl delete -f config/

##@ Helm

.PHONY: helm-lint
helm-lint: ## Lint the Helm chart.
	@helm lint charts/karpenter-provider-bizflycloud

.PHONY: helm-template
helm-template: ## Template the Helm chart.
	@helm template karpenter-provider charts/karpenter-provider-bizflycloud --debug

.PHONY: helm-package
helm-package: ## Package the Helm chart.
	@mkdir -p charts/dist
	@helm package charts/karpenter-provider-bizflycloud -d charts/dist --version $(VERSION)

.PHONY: helm-index
helm-index: ## Create or update the Helm chart index.
	@helm repo index charts/dist --url https://bizflycloud.github.io/karpenter-provider-bizflycloud/charts

##@ Release

.PHONY: version
version: ## Print the version.
	@echo $(VERSION)

.PHONY: crds
crds: ## Validate and copy CRDs to the Helm chart.
	@echo "Copying validated CRDs to Helm chart..."
	@cp config/crd/bizflycloud.karpenter.sh_providerconfigs.yaml charts/karpenter-provider-bizflycloud/templates/crd.yaml

.PHONY: release
release: test crds helm-lint docker-build docker-push helm-package helm-index ## Create a new release.
	@echo "Creating release $(VERSION)"
	@echo "  - Docker image pushed: cr-hn-1.bizflycloud.vn/31ff9581861a4d0ea4df5e7dda0f665d/karpenter-provider-bizflycloud:$(VERSION)"
	@echo "  - Helm chart packaged: charts/dist/karpenter-provider-bizflycloud-$(VERSION).tgz"
	@echo "Release complete!"

.PHONY: release-tag
release-tag: ## Create and push a git tag for a release.
	git tag -a v$(VERSION) -m "Release $(VERSION)"
	git push origin v$(VERSION)