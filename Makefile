# ============================================================
#  provider-opsmanager — developer Makefile
#
#  Wraps the inner-loop for building, packaging, loading into the
#  local kind cluster, and (re)deploying the provider.
#
#  Assumes the dev lab (kind + Crossplane + Ops Manager) is already
#  running. See /home/crossplane-faust/Makefile for `make all` to
#  bootstrap the lab.
#
#  Typical loop:
#    make redeploy   # generate + build + image + load + restart pod
#    make logs       # follow provider pod logs
# ============================================================

SHELL := /bin/bash

# ── Configuration ────────────────────────────────────────────
CLUSTER_NAME       ?= local-dev
PROVIDER_NAMESPACE ?= crossplane-system
PROVIDER_NAME      ?= provider-opsmanager

# Image coordinates. For local dev we tag :dev and load into kind.
# For a real push, override REGISTRY and VERSION on the make invocation.
REGISTRY           ?= ghcr.io/faust1no
IMAGE_NAME         ?= opsmanager-crossplane-provider
VERSION            ?= dev
IMAGE              := $(REGISTRY)/$(IMAGE_NAME):$(VERSION)
XPKG_FILE          := provider-opsmanager-$(VERSION).xpkg

# Toolchain. Override these if you have non-PATH binaries.
GO                 ?= go
DOCKER             ?= docker
KIND               ?= kind
KUBECTL            ?= kubectl
CROSSPLANE_CLI     ?= crossplane

# ── Colors ───────────────────────────────────────────────────
RESET  := \033[0m
BOLD   := \033[1m
GREEN  := \033[32m
YELLOW := \033[33m
CYAN   := \033[36m
RED    := \033[31m

define log
	@printf '$(CYAN)$(BOLD)[$(1)]$(RESET)  %s\n' $(2)
endef

.PHONY: help generate build image xpkg load install uninstall \
        redeploy logs status clean lint test vendor

# ============================================================
#  HELP
# ============================================================
help:
	@echo ""
	@printf '$(BOLD)Usage:$(RESET) make [target]\n\n'
	@printf '$(BOLD)Inner-loop:$(RESET)\n'
	@printf '  $(GREEN)redeploy$(RESET)    Build, package, load into kind, restart provider pod (the common dev loop)\n'
	@printf '  $(GREEN)logs$(RESET)        Follow logs from the running provider pod\n'
	@printf '  $(GREEN)status$(RESET)      Show provider package and pod state\n'
	@echo ""
	@printf '$(BOLD)Code generation & build:$(RESET)\n'
	@printf '  $(GREEN)generate$(RESET)    Run controller-gen + angryjet (CRDs, deepcopy, methodsets)\n'
	@printf '  $(GREEN)build$(RESET)       go build all packages (sanity check)\n'
	@printf '  $(GREEN)image$(RESET)       Build the controller container image\n'
	@printf '  $(GREEN)xpkg$(RESET)        Build the Crossplane xpkg package\n'
	@echo ""
	@printf '$(BOLD)Cluster integration:$(RESET)\n'
	@printf '  $(GREEN)load$(RESET)        Load the controller image into kind cluster $(CLUSTER_NAME)\n'
	@printf '  $(GREEN)install$(RESET)     Install the Provider CR pointing at the local image\n'
	@printf '  $(GREEN)uninstall$(RESET)   Delete the Provider CR (leaves CRDs by default)\n'
	@echo ""
	@printf '$(BOLD)Quality:$(RESET)\n'
	@printf '  $(GREEN)lint$(RESET)        Run golangci-lint\n'
	@printf '  $(GREEN)test$(RESET)        Run go test\n'
	@printf '  $(GREEN)vendor$(RESET)      Refresh vendor/ directory\n'
	@echo ""
	@printf '$(BOLD)Overrides (env vars):$(RESET)\n'
	@printf '  VERSION=$(YELLOW)$(VERSION)$(RESET) REGISTRY=$(YELLOW)$(REGISTRY)$(RESET) CLUSTER_NAME=$(YELLOW)$(CLUSTER_NAME)$(RESET)\n'
	@echo ""

# ============================================================
#  CODE GENERATION
# ============================================================
generate:
	$(call log,GEN,'Running controller-gen (deepcopy + CRDs)...')
	@$(GO) run sigs.k8s.io/controller-tools/cmd/controller-gen \
		object:headerFile=hack/boilerplate.go.txt paths=./apis/...
	@$(GO) run sigs.k8s.io/controller-tools/cmd/controller-gen \
		crd paths=./apis/... output:crd:artifacts:config=package/crds
	$(call log,GEN,'Running angryjet (managed/PCU method sets)...')
	@$(GO) run github.com/crossplane/crossplane-tools/cmd/angryjet \
		generate-methodsets --header-file=hack/boilerplate.go.txt ./apis/...
	$(call log,OK,'Code generation complete.')

# ============================================================
#  BUILD
# ============================================================
build:
	$(call log,BUILD,'Compiling Go packages...')
	@$(GO) build ./...
	$(call log,OK,'Build succeeded.')

# Build the controller image. Uses the project Dockerfile.
image: build
	$(call log,IMAGE,'Building $(IMAGE)...')
	@$(DOCKER) build -t $(IMAGE) .
	$(call log,OK,'Image $(IMAGE) ready.')

# Build the Crossplane xpkg, embedding the controller image we just built.
xpkg: image
	$(call log,XPKG,'Building xpkg $(XPKG_FILE) (embed-runtime-image=$(IMAGE))...')
	@$(CROSSPLANE_CLI) xpkg build \
		--package-root=package \
		--embed-runtime-image=$(IMAGE) \
		--package-file=$(XPKG_FILE)
	$(call log,OK,'xpkg written to $(XPKG_FILE).')

# ============================================================
#  CLUSTER INTEGRATION
# ============================================================

# Load the controller image into the kind cluster so the Provider CR
# can pull it without going through a registry.
load: image
	$(call log,LOAD,'Loading $(IMAGE) into kind cluster $(CLUSTER_NAME)...')
	@$(KIND) load docker-image $(IMAGE) --name $(CLUSTER_NAME)
	$(call log,OK,'Image loaded.')

# Install the Provider CR. The DeploymentRuntimeConfig forces the controller
# pod to use our locally-loaded image with IfNotPresent so kubelet does not
# try to pull from a remote registry.
install:
	$(call log,INSTALL,'Applying Provider $(PROVIDER_NAME) referencing $(IMAGE)...')
	@printf '%s\n' \
		'apiVersion: pkg.crossplane.io/v1beta1' \
		'kind: DeploymentRuntimeConfig' \
		'metadata:' \
		'  name: $(PROVIDER_NAME)-local' \
		'spec:' \
		'  deploymentTemplate:' \
		'    spec:' \
		'      selector: {}' \
		'      template:' \
		'        spec:' \
		'          containers:' \
		'            - name: package-runtime' \
		'              image: $(IMAGE)' \
		'              imagePullPolicy: IfNotPresent' \
		'---' \
		'apiVersion: pkg.crossplane.io/v1' \
		'kind: Provider' \
		'metadata:' \
		'  name: $(PROVIDER_NAME)' \
		'spec:' \
		'  package: $(IMAGE)' \
		'  packagePullPolicy: IfNotPresent' \
		'  runtimeConfigRef:' \
		'    name: $(PROVIDER_NAME)-local' \
		| $(KUBECTL) apply -f -
	$(call log,OK,'Provider $(PROVIDER_NAME) applied.')

uninstall:
	$(call log,UNINSTALL,'Deleting Provider $(PROVIDER_NAME)...')
	@$(KUBECTL) delete provider.pkg.crossplane.io $(PROVIDER_NAME) --ignore-not-found
	@$(KUBECTL) delete deploymentruntimeconfig.pkg.crossplane.io $(PROVIDER_NAME)-local --ignore-not-found
	$(call log,OK,'Provider uninstalled. (CRDs left in place — kubectl get crd | grep opsmanager)')

# The dev inner-loop: regenerate, build, load image, and bounce the pod.
# Restarts the deployment so the new image is picked up immediately.
redeploy: generate image load
	$(call log,REDEPLOY,'Restarting provider pod to pick up new image...')
	@DEP=$$($(KUBECTL) get deployment -n $(PROVIDER_NAMESPACE) \
		-l pkg.crossplane.io/provider=$(PROVIDER_NAME) -o name 2>/dev/null | head -1); \
	if [ -n "$$DEP" ]; then \
		$(KUBECTL) rollout restart -n $(PROVIDER_NAMESPACE) $$DEP; \
		$(KUBECTL) rollout status -n $(PROVIDER_NAMESPACE) $$DEP --timeout=90s; \
	else \
		printf '$(YELLOW)$(BOLD)[WARN]$(RESET)  No provider deployment found — run "make install" first.\n'; \
	fi
	$(call log,OK,'Redeploy complete.')

# ============================================================
#  OBSERVABILITY
# ============================================================
logs:
	@POD=$$($(KUBECTL) get pod -n $(PROVIDER_NAMESPACE) \
		-l pkg.crossplane.io/provider=$(PROVIDER_NAME) -o name 2>/dev/null | head -1); \
	if [ -z "$$POD" ]; then \
		printf '$(RED)$(BOLD)[ERROR]$(RESET) No provider pod found in $(PROVIDER_NAMESPACE).\n'; \
		exit 1; \
	fi; \
	$(KUBECTL) logs -n $(PROVIDER_NAMESPACE) $$POD -c package-runtime -f

status:
	@echo ""
	@printf '$(BOLD)Provider package$(RESET)\n'
	@$(KUBECTL) get provider.pkg.crossplane.io $(PROVIDER_NAME) 2>/dev/null \
		|| echo '  (Provider CR not installed)'
	@echo ""
	@printf '$(BOLD)Provider pod$(RESET)\n'
	@$(KUBECTL) get pods -n $(PROVIDER_NAMESPACE) \
		-l pkg.crossplane.io/provider=$(PROVIDER_NAME) 2>/dev/null \
		|| echo '  (No pod found)'
	@echo ""
	@printf '$(BOLD)Installed CRDs$(RESET)\n'
	@$(KUBECTL) get crd -l app.kubernetes.io/part-of=crossplane 2>/dev/null \
		| grep opsmanager.crossplane.io || echo '  (No opsmanager CRDs installed)'
	@echo ""

# ============================================================
#  QUALITY
# ============================================================
lint:
	$(call log,LINT,'Running golangci-lint...')
	@$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint run ./...

test:
	$(call log,TEST,'Running go test...')
	@$(GO) test ./...

vendor:
	$(call log,VENDOR,'Refreshing vendor/...')
	@$(GO) mod tidy
	@$(GO) mod vendor
	$(call log,OK,'vendor/ refreshed.')

clean:
	$(call log,CLEAN,'Removing build artifacts...')
	@rm -f provider-opsmanager-*.xpkg
	$(call log,OK,'Clean complete.')
