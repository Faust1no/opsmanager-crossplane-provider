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

## 1 — Deploy the Lab

The `Makefile` at the root of this repository bootstraps a complete local stack:

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

## 2 — Run the Provider Locally

See [TESTING.md](./TESTING.md) for the full walkthrough.

The short version:

```bash
# 1. Install CRDs into the cluster
kubectl apply -f package/crds/

# 2. Create the Ops Manager credentials secret
kubectl create secret generic opsmanager-credentials \
  -n crossplane-system \
  --from-literal=credentials='{"publicKey":"<your-public-key>","privateKey":"<your-private-key>"}'

# 3. Apply the ProviderConfig
kubectl apply -f examples/providerconfig.yaml

# 4. Run the provider binary (watches the cluster, reconciles CRs)
go run ./cmd/provider/main.go
```

The provider runs out-of-cluster using your local kubeconfig.
No Docker image is needed for local development.

---

## 3 — Build and Push the Package

A Crossplane package (`xpkg`) is an OCI image that bundles the provider runtime container
and the CRD manifests. There are two ways to build it.

### Option A — Crossplane CLI (recommended)

This is the standard approach. The CLI builds and pushes an `xpkg` directly.

```bash
VERSION=v1.1.0
IMAGE=ghcr.io/faust1no/opsmanager-crossplane-provider

# 1. Build the provider runtime image
docker build -t ${IMAGE}:${VERSION} .

# 2. Push the runtime image
docker push ${IMAGE}:${VERSION}

# 3. Update the image reference in the package manifest
#    Edit package/crossplane.yaml and set:
#      spec.controller.image: ghcr.io/faust1no/opsmanager-crossplane-provider:<VERSION>

# 4. Build the xpkg (bundles CRDs + runtime image reference)
crossplane xpkg build \
  --package-root=package \
  --output=provider-opsmanager-${VERSION}.xpkg

# 5. Push the xpkg to the registry
crossplane xpkg push \
  --package=provider-opsmanager-${VERSION}.xpkg \
  ${IMAGE}:${VERSION}
```

After pushing, log out to remove the stored credential:

```bash
docker logout ghcr.io
```

> **After pushing**, update the provider version in the GitOps repository so the new
> package is picked up by the cluster. Change the `spec.package` tag on the `Provider`
> object to match the version you just pushed, then open a PR in that repo.

### Option B — Docker only

Use this if the Crossplane CLI is not available in the build environment.

```bash
VERSION=v1.1.0
IMAGE=ghcr.io/faust1no/opsmanager-crossplane-provider

# Build the provider runtime image
docker build -t ${IMAGE}:${VERSION} .
docker push ${IMAGE}:${VERSION}

docker logout ghcr.io
```

> Note: without the Crossplane CLI you cannot produce an `xpkg`.
> The runtime image alone can be referenced in a manually authored `Provider` CR,
> but the CRDs must be installed separately (`kubectl apply -f package/crds/`).

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
