# Local Development Guide

This guide walks a new engineer through standing up the full local lab, running the provider
out-of-cluster, and building a publishable Crossplane package.

The lab runs entirely inside a [kind](https://kind.sigs.k8s.io/) cluster on your laptop.
No cloud account or VPN is required.

---

## Prerequisites

Install the following tools before running anything:

| Tool | Minimum version | Install |
|------|----------------|---------|
| Docker | 24+ | https://docs.docker.com/get-docker/ |
| kind | 0.22+ | `brew install kind` |
| kubectl | 1.29+ | `brew install kubectl` |
| helm | 3.14+ | `brew install helm` |
| Crossplane CLI | 1.15+ | see below |
| Go | 1.21+ | https://go.dev/dl/ |

Install the Crossplane CLI:

```bash
curl -sL "https://raw.githubusercontent.com/crossplane/crossplane/master/install.sh" | sh
sudo mv crossplane /usr/local/bin/
```

---

> **Two Makefiles, two jobs.** The repo uses two separate Makefiles that target
> different concerns. Keep them straight as you read on:
>
> | Makefile | Location | What it does |
> |---|---|---|
> | **Lab Makefile** | `/home/crossplane-faust/Makefile` (outside this repo) | Bootstraps the surrounding lab — kind cluster, Crossplane install, Ops Manager, MinIO, LDAP, etc. Run once per laptop. |
> | **Provider Makefile** | `Makefile` (repo root) | Builds / packages / installs *this* provider into the lab cluster. Run on every code change. |
>
> Sections 1 covers the lab Makefile. Section 2 covers the provider Makefile and the dev inner loop.

## 1 — Deploy the Lab

The lab `Makefile` at `/home/crossplane-faust/Makefile` bootstraps a complete local stack:

| Component | Version | Purpose |
|-----------|---------|---------|
| kind cluster | — | Single-node local Kubernetes |
| MongoDB Controllers for Kubernetes | 1.7.0 | Manages `MongoDBOpsManager` CR |
| MongoDB Ops Manager | 7.0.11 | The system this provider targets |
| Crossplane | 1.15.0 | Provider runtime |
| MinIO | latest | S3-compatible object storage (blockstore/oplog backend) |
| GLAuth | latest | Lightweight LDAP server for LDAP group mapping tests |
| Headlamp | latest | Kubernetes UI — useful for inspecting CRs |

### Quick start

```bash
make all
```

This runs preflight checks, then provisions every component in order.
Ops Manager takes 8–12 minutes to reach `Running` phase — the target waits automatically.

### Individual targets

```bash
make cluster          # Create the kind cluster only
make mck              # Install MongoDB Controllers for Kubernetes operator
make ops-manager      # Deploy MongoDBOpsManager CR (includes its AppDB replica set)
make crossplane       # Install Crossplane
make minio            # Deploy MinIO + create the default bucket
make glauth           # Deploy GLAuth LDAP server
make headlamp         # Deploy Headlamp dashboard

make status           # Print status of every component and access URLs
make teardown         # Delete the cluster and all generated files
```

### Port reference

| Service | External URL | Notes |
|---------|-------------|-------|
| Ops Manager | http://localhost:32001 | Login: `admin@example.com` / `OpsManager@SecurePass123!` |
| MinIO S3 API | http://localhost:30090 | Used as S3 endpoint in blockstore/oplog CRs |
| MinIO Console | http://localhost:30091 | Login: `minioadmin` / `minioadmin123` |
| LDAP | `localhost:30092` | Bind DN: `cn=serviceaccount,dc=opsmanager,dc=local` / `ldapadmin` |
| Headlamp | http://localhost:30080 | Run `make headlamp-token` to get the auth token |

### Test MongoDB cluster (optional)

To spin up an actual MongoDB replica set managed by Ops Manager — useful for end-to-end
backup testing — first create an API key in the Ops Manager UI, then:

```bash
MCK_TEST_PUBLIC_KEY=<your-public-key> \
MCK_TEST_PRIVATE_KEY=<your-private-key> \
MCK_TEST_ORG_ID=<your-org-id> \
make test-mongodb
```

Watch the replica set reach `Running`:

```bash
kubectl get mongodb test-rs -n mongodb -w
```

### Makefile

The full Makefile is included below for reference. It lives at the repository root.

<details>
<summary>Expand Makefile</summary>

```makefile
# ============================================================
#  Makefile — Local Dev Stack Bootstrap
#  Installs: kind cluster | MongoDB Controllers for K8s v1.7.0
#            Ops Manager | Crossplane | Headlamp | MinIO
# ============================================================

SHELL := /bin/bash

CLUSTER_NAME        ?= local-dev
KIND_CONFIG         ?= kind-config.yaml
KUBECONFIG          ?= $(HOME)/.kube/config

MCK_VERSION         := 1.7.0
MCK_NAMESPACE       := mongodb
MONGODB_HELM_REPO   := https://mongodb.github.io/helm-charts

OPS_MANAGER_VERSION := 7.0.11
OPS_MANAGER_APPDB   := 6.0.14-ubi8

CROSSPLANE_NAMESPACE := crossplane-system
CROSSPLANE_VERSION   := 1.15.0

HEADLAMP_NAMESPACE   := headlamp
MINIO_NAMESPACE      := minio
MINIO_ROOT_USER      := minioadmin
MINIO_ROOT_PASSWORD  := minioadmin123
MINIO_BUCKET         := ops-manager-blockstore
GLAUTH_NAMESPACE     := glauth

# Override these with your own Ops Manager API key for test-mongodb target
MCK_TEST_PUBLIC_KEY  ?= <your-public-key>
MCK_TEST_PRIVATE_KEY ?= <your-private-key>
MCK_TEST_ORG_ID      ?= <your-org-id>
MCK_TEST_PROJECT     ?= test-backup
MCK_TEST_RS_NAME     ?= test-rs
MCK_TEST_RS_VERSION  ?= 6.0.5
```

> See the full Makefile source at the repository root for all target implementations.

</details>

---

## 2 — Provider Makefile

The provider Makefile at the repo root wraps every step of the dev loop so you
do not have to remember `go run sigs.k8s.io/controller-tools/...`,
`crossplane xpkg build --package-root=... --embed-runtime-image=...`,
`kind load docker-image ...`, and so on. Run `make help` for the full list.

### 2.1 The everyday inner loop

After editing code, the one command you usually want is:

```bash
make redeploy   # generate → build image → kind load → restart provider pod
make logs       # follow logs from the running provider
```

`make redeploy` chains `generate → image → load`, then restarts the provider
deployment so kubelet picks up the new image. Because the deployment is created
by `make install` with `imagePullPolicy: IfNotPresent` and tagged `:dev`, kubelet
uses the image already loaded into kind instead of pulling from a registry.

### 2.2 One-time install (after the lab is up)

The first time you run the provider against a fresh lab cluster:

```bash
make image      # builds ghcr.io/faust1no/opsmanager-crossplane-provider:dev
make load       # loads it into kind cluster `local-dev`
make install    # applies a Provider CR + DeploymentRuntimeConfig pinned to :dev
make status     # confirms the provider package is healthy
```

After this, the everyday loop above is enough — no need to re-run `make install`
unless you delete the Provider CR.

### 2.3 Out-of-cluster run (alternative)

For the fastest iteration on controller logic (no Docker build, no restart
delay), skip `make install` and run the binary against the cluster directly:

```bash
kubectl apply -f package/crds/
kubectl apply -f examples/providerconfig.yaml

go run ./cmd/provider/main.go --debug
```

The provider uses your local kubeconfig and reconciles CRs in the cluster.
Useful for stepping through code in a debugger.

### 2.4 Target reference

| Target | Use when |
|---|---|
| `make generate` | You changed types under `apis/` and need updated CRDs/deepcopy/methodsets |
| `make build` | Sanity check that `go build ./...` is clean |
| `make image` | Build the controller container image at `:dev` |
| `make xpkg` | Build a releasable `xpkg` (see release section below) |
| `make load` | Push the local image into the kind cluster |
| `make install` | Apply the Provider CR + DeploymentRuntimeConfig (`:dev`, IfNotPresent) |
| `make uninstall` | Remove the Provider CR (CRDs are left in place) |
| `make redeploy` | Inner loop: regenerate → image → load → bounce pod |
| `make logs` | Tail provider pod logs |
| `make status` | Show Provider package, pod, and installed CRDs |
| `make lint` | Run `golangci-lint` |
| `make test` | Run `go test ./...` |
| `make vendor` | Refresh `vendor/` after `go.mod` changes |
| `make clean` | Delete stale `provider-opsmanager-*.xpkg` artifacts |

### 2.5 Overrides

Any of these can be set on the make command line:

```bash
make redeploy CLUSTER_NAME=staging-dev
make xpkg     VERSION=v2.0.3
make image    REGISTRY=registry.internal/platform
```

| Variable | Default | Notes |
|---|---|---|
| `CLUSTER_NAME` | `local-dev` | Matches the lab Makefile default |
| `VERSION` | `dev` | Override for releases |
| `REGISTRY` | `ghcr.io/faust1no` | The image registry path |
| `IMAGE_NAME` | `opsmanager-crossplane-provider` | Image repo name |
| `PROVIDER_NAMESPACE` | `crossplane-system` | Where the provider pod runs |

---

## 3 — Build and Push a Release

A Crossplane package (`xpkg`) is an OCI image that bundles the provider runtime
container and the CRD manifests. The provider Makefile builds it in one step.

```bash
# Build the controller image + xpkg at a release version.
make xpkg VERSION=v2.0.3

# Push the controller image to the registry.
docker push ghcr.io/faust1no/opsmanager-crossplane-provider:v2.0.3

# Push the xpkg.
crossplane xpkg push \
  -f provider-opsmanager-v2.0.3.xpkg \
  ghcr.io/faust1no/opsmanager-crossplane-provider:v2.0.3

docker logout ghcr.io
```

Before tagging, also bump `spec.controller.image` in `package/crossplane.yaml`
to match the new version, otherwise the xpkg will embed an old image reference.

> **After pushing**, update the provider version in the GitOps repository so the
> new package is picked up by the cluster. Change the `spec.package` tag on the
> `Provider` object to match the version you just pushed, then open a PR.

---

## 4 — CHANGELOG Requirement

**Every pull request must update `CHANGELOG.md`.** A CI check enforces this —
the PR will fail if `CHANGELOG.md` is not modified.

Format: add an entry under `[Unreleased]` following the
[Keep a Changelog](https://keepachangelog.com/en/1.0.0/) convention:

```markdown
## [Unreleased]
### Fixed
- Short description of what you changed and why.
```

Valid section headers: `Added`, `Changed`, `Fixed`, `Removed`, `Security`.

When a version is released, the `[Unreleased]` block is renamed to `[X.Y.Z] - YYYY-MM-DD`.
