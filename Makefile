# Stratos Makefile

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOVET=$(GOCMD) vet
GOFMT=$(GOCMD) fmt
GOMOD=$(GOCMD) mod
BINARY_NAME=stratos

# Tool versions
CONTROLLER_GEN_VERSION=v0.16.5
ENVTEST_VERSION=release-0.17

# Build variables
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)"

# Image variables
IMG ?= ghcr.io/stratos-sh/stratos:$(VERSION)

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: fmt
fmt: ## Run go fmt against code
	$(GOFMT) ./...

.PHONY: vet
vet: ## Run go vet against code
	$(GOVET) ./...

.PHONY: lint
lint: golangci-lint ## Run golangci-lint against code
	$(GOLANGCI_LINT) run

.PHONY: test
test: ## Run tests
	$(GOTEST) -coverprofile=coverage.out ./...

.PHONY: test-integration
test-integration: envtest ## Run integration tests
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" $(GOTEST) ./tests/integration/... -v -tags=integration

.PHONY: coverage
coverage: test ## Generate coverage report
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

##@ Build

.PHONY: build
build: fmt vet ## Build the binary
	$(GOBUILD) $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/stratos

.PHONY: run
run: fmt vet ## Run the controller locally
	$(GOCMD) run ./cmd/stratos/main.go

.PHONY: docker-build
docker-build: ## Build docker image
	docker buildx build --platform linux/$(shell go env GOARCH) -t $(IMG) --load .

.PHONY: docker-push
docker-push: ## Push docker image
	docker push $(IMG)

##@ Code Generation

.PHONY: generate
generate: controller-gen ## Generate code (deepcopy, etc.)
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: manifests
manifests: controller-gen ## Generate CRD and RBAC manifests
	$(CONTROLLER_GEN) crd paths="./..." output:crd:artifacts:config=deploy/charts/stratos/crds

##@ Deployment

.PHONY: install
install: manifests ## Install CRDs into the K8s cluster
	kubectl apply -f deploy/charts/stratos/crds

.PHONY: uninstall
uninstall: ## Uninstall CRDs from the K8s cluster
	kubectl delete -f deploy/charts/stratos/crds

HELM_RELEASE ?= stratos
HELM_NAMESPACE ?= stratos-system

.PHONY: deploy
deploy: ## Deploy controller via Helm
	helm upgrade --install $(HELM_RELEASE) deploy/charts/stratos \
		--namespace $(HELM_NAMESPACE) --create-namespace \
		--set clusterName=$(CLUSTER_NAME)

.PHONY: undeploy
undeploy: ## Undeploy controller via Helm
	helm uninstall $(HELM_RELEASE) --namespace $(HELM_NAMESPACE)

##@ Dependencies

.PHONY: deps
deps: ## Download dependencies
	$(GOMOD) download
	$(GOMOD) tidy

##@ Tool Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
GOLANGCI_LINT ?= $(LOCALBIN)/golangci-lint
ENVTEST ?= $(LOCALBIN)/setup-envtest

## Tool Versions
CONTROLLER_TOOLS_VERSION ?= $(CONTROLLER_GEN_VERSION)
GOLANGCI_LINT_VERSION ?= v1.55.2
ENVTEST_K8S_VERSION ?= 1.28.0

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary
$(CONTROLLER_GEN): $(LOCALBIN)
	@test -s $(LOCALBIN)/controller-gen && $(LOCALBIN)/controller-gen --version | grep -q $(CONTROLLER_TOOLS_VERSION) || \
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary
$(GOLANGCI_LINT): $(LOCALBIN)
	@test -s $(LOCALBIN)/golangci-lint || \
	GOBIN=$(LOCALBIN) go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

.PHONY: envtest
envtest: $(ENVTEST) ## Downlo-ad envtest-setup locally if necessary
$(ENVTEST): $(LOCALBIN)
	@test -s $(LOCALBIN)/setup-envtest || \
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

##@ Clean

.PHONY: clean
clean: ## Clean build artifacts
	rm -rf bin/
	rm -f coverage.out coverage.html
