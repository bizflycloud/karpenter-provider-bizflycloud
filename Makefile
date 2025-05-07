# Image URL to use all building/pushing image targets
IMG ?= fevirtus/karpenter-provider-bizflycloud:latest
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.24.2

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

MODULE := github.com/bizflycloud/karpenter-provider-bizflycloud
APIS := ./pkg/apis/v1
BOILERPLATE := ./hack/boilerplate.go.txt
CRD_OUTPUT := ./config/crd
CONTROLLER_GEN := $(shell which controller-gen)

.PHONY: generate
generate: deepcopy crd
deepcopy:
	@echo "🧬 Generating deepcopy methods..."
	$(CONTROLLER_GEN) object:headerFile="$(BOILERPLATE)" paths="$(APIS)"
crd:
	@echo "📦 Generating CRDs..."
	$(CONTROLLER_GEN) crd:crdVersions=v1 object:headerFile="$(BOILERPLATE)" paths="$(APIS)" output:crd:dir="$(CRD_OUTPUT)"
clean:
	@echo "🧹 Cleaning generated files..."
	find $(APIS) -name "zz_generated.deepcopy.go" -delete
	rm -rf $(CRD_OUTPUT)/*.yaml

.PHONY: all
all: build

.PHONY: manifests
manifests: ## Generate CRDs and RBAC.
	@echo "Validating CRDs..."
	@kubectl validate --schema $(shell kubectl api-resources --api-group=apiextensions.k8s.io -o name | grep customresourcedefinition) -f config/crd/karpenter.bizflycloud.com_providerconfigs.yaml

##@ Build

.PHONY: build
build: fmt vet ## Build manager binary.
	go build $(LDFLAGS) -o bin/karpenter-provider-bizflycloud cmd/karpenter-provider-bizflycloud/main.go

.PHONY: run
run: fmt vet ## Run from your host.
	go run $(LDFLAGS) ./cmd/karpenter-provider-bizflycloud/main.go

.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	docker build --platform=linux/amd64 --build-arg VERSION=$(VERSION) -t ${IMG} .

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
