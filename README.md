# provider-opsmanager

A [Crossplane](https://crossplane.io) provider for [MongoDB Ops Manager](https://www.mongodb.com/docs/ops-manager/current/).
Manages backup infrastructure declaratively via Kubernetes CRs.

## Crossplane concepts

Crossplane extends Kubernetes so that external infrastructure can be managed
with the same declarative model as pods and services. These are the core
concepts this provider builds on.

**Provider**
A provider is a Kubernetes controller that knows how to talk to one external
API. This repo _is_ a provider — it targets the MongoDB Ops Manager REST API.
When installed, it runs as a pod inside the cluster and watches for CRs.

**ClusterProviderConfig** (`apis/v1beta1/providerconfig_types.go`)
Holds the connection details shared by managed resources: the Ops Manager base
URL and a reference to a Kubernetes Secret containing the API key pair.
Cluster-scoped and referenced by every managed resource in this provider via
`providerConfigRef: { kind: ClusterProviderConfig, name: ... }`. One
`ClusterProviderConfig` maps 1:1 to one Ops Manager instance, matching Ops
Manager's global-singleton data model.

```yaml
apiVersion: opsmanager.crossplane.io/v1beta1
kind: ClusterProviderConfig
metadata:
  name: default
spec:
  baseURL: http://ops-manager:8080/
  credentials:
    source: Secret
    publicKeySecretRef:
      namespace: crossplane-system
      name: opsmanager-credentials
      key: publicKey
    privateKeySecretRef:
      namespace: crossplane-system
      name: opsmanager-credentials
      key: privateKey
```

**Managed Resource**
A managed resource is a CR that represents exactly one external object.
The provider reconciles it continuously — creating, updating, or deleting the
external object to match the spec. Each managed resource has:

- `spec.forProvider` — the desired state you declare (what you want). On the first
  reconcile after adopting an existing resource, any optional fields left empty are
  automatically back-filled from the API — so the spec reflects the full deployed
  configuration without you having to know it upfront.
- `status.atProvider` — read-only state observed from the API (IDs, usage metrics)
- `status.conditions` — `Ready` and `Synced` conditions set by the reconciler

**Reconcile loop**
On every reconcile the provider calls four methods in order:

| Method | When called | What it does |
|---|---|---|
| `Observe` | always | GET the external resource; returns whether it exists and is up to date |
| `Create` | resource does not exist | POST to create it |
| `Update` | resource exists but drifted from spec | PUT to reconcile the diff |
| `Delete` | CR has a deletion timestamp | DELETE the external resource, then release the finalizer |

The provider re-runs this loop every minute (configurable) so external drift
is detected and corrected automatically.

**External name**
Crossplane tracks the external ID of a resource in the annotation
`crossplane.io/external-name`. For this provider that is the `id` field from
`spec.forProvider` (e.g. `minio-blockstore`). It is set during `Create` and
used as the key for `Get`/`Update`/`Delete` calls.

**Finalizer**
Crossplane adds `finalizer.managedresource.crossplane.io` to every managed
resource. This prevents the CR from being deleted from Kubernetes until the
provider has successfully cleaned up the external resource (or explicitly
decided not to, as with `BackupDaemon`).

---

## Managed resources

| Kind | Ops Manager resource |
|---|---|
| `OpsManagerProject` | Project with LDAP group mappings |
| `S3Blockstore` | S3-backed snapshot store |
| `S3OplogStore` | S3-backed oplog store (shared across clusters) |
| `BackupDaemon` | Backup daemon configuration |

## Requirements

| Tool | Minimum version |
|---|---|
| Go | 1.21 |
| Docker | any recent |
| Crossplane CLI | v1.14+ |

Install the Crossplane CLI:

```bash
curl -sL https://raw.githubusercontent.com/crossplane/crossplane/master/install.sh | sh
sudo mv crossplane /usr/local/bin/
```

## Building the package

The provider ships as a single `.xpkg` OCI artifact — controller binary, CRDs,
and metadata in one file. Build it in three steps:

### Step 1 — Vendor dependencies

```bash
go mod vendor
```

This is required because `go.mod` uses a local `replace` directive pointing at
the SDK fork (`../go-client-mongodb-ops-manager`). Vendoring copies it into the
repo so the Docker build context is self-contained.

### Step 2 — Build the controller image

```bash
docker build -t provider-opsmanager:<version> .
```

> If `gcr.io` is unreachable in your build environment, pull and save
> `gcr.io/distroless/static:nonroot` on a connected machine first, then load it
> before running the build.

### Step 3 — Build the `.xpkg`

```bash
crossplane xpkg build \
  --package-root=./package \
  --embed-runtime-image=provider-opsmanager:<version> \
  --package-file=provider-opsmanager.xpkg
```

## Pulling the package from the registry

The latest `.xpkg` is published to GitHub Container Registry. To pull and save
it as a tar file for transfer to an air-gapped environment:

```bash
# Pull and save as a gzipped tar
docker pull ghcr.io/faust1no/opsmanager-crossplane-provider:<version>
docker save ghcr.io/faust1no/opsmanager-crossplane-provider:<version> | gzip > provider-opsmanager-<version>.tar.gz
```

Transfer the file to your target machine, then load and extract the `.xpkg`:

```bash
# On the target machine — load the image
docker load < provider-opsmanager-<version>.tar.gz

# Save as .xpkg using the crossplane CLI
crossplane xpkg build \
  --package-root=./package \
  --embed-runtime-image=ghcr.io/faust1no/opsmanager-crossplane-provider:<version> \
  --package-file=provider-opsmanager-<version>.xpkg
```

Or if you just want to download the raw OCI artifact directly without Docker:

```bash
crossplane xpkg pull ghcr.io/faust1no/opsmanager-crossplane-provider:<version> \
  --package-file=provider-opsmanager-<version>.xpkg
```

---

## Deploying in an air-gapped cluster

Push the package to your internal registry:

```bash
crossplane xpkg push \
  --package-files=provider-opsmanager.xpkg \
  your-internal-registry/provider-opsmanager:<version>
```

Install the provider:

```bash
kubectl apply -f - <<EOF
apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: provider-opsmanager
spec:
  package: your-internal-registry/provider-opsmanager:v0.1.0
  packagePullPolicy: IfNotPresent
EOF
```

Watch it come up:

```bash
kubectl get provider provider-opsmanager -w
# INSTALLED=True, HEALTHY=True
```

Then follow [TESTING.md](TESTING.md) to configure a `ClusterProviderConfig` and apply managed resources.

## Adopting existing Ops Manager resources

To adopt an existing resource, apply a CR whose adoption-key field matches the
resource in Ops Manager. On the first reconcile `Observe` finds it and the
provider takes over without recreating it. Any optional fields you leave empty
are back-filled from the API on that first observe.

For each kind, the **adoption key** is the `spec.forProvider` field the
controller uses to look the resource up. The **must-set** column lists the
fields that have no API default — the request fails without them.

| Kind | Adoption key | Must set in `spec.forProvider` |
|---|---|---|
| `OpsManagerProject`   | `name`    | `name`, `orgId` |
| `S3Blockstore`        | `id`      | `id`, `s3BucketEndpoint`, `s3BucketName`, `s3AuthMethod`. When `s3AuthMethod: KEYS`: `awsAccessKey`, `awsSecretKeySecretRef`. |
| `S3OplogStore`        | `id`      | same as `S3Blockstore` |
| `BackupDaemon`        | `machine` | `machine` |

All four managed resources are cluster-scoped and reference
`spec.providerConfigRef.kind: ClusterProviderConfig`. Ops Manager holds one
config per external identifier globally (per project name, per store id, per
daemon machine), so a cluster-scoped `ClusterProviderConfig`-to-OM 1:1 mapping
matches the underlying data model exactly.

See [ADOPTION.md](ADOPTION.md) for Helm-style chart templates with the minimum
adoption spec per kind.

---

## Development

- [TESTING.md](TESTING.md) — run the provider out-of-cluster against a local kind cluster
- [PROVIDER.md](PROVIDER.md) — architecture, controller design, and SDK changes
- [CONVENTIONS.md](CONVENTIONS.md) — conventions for adding new managed resources
- [../go-client-mongodb-ops-manager/CHANGES.md](../go-client-mongodb-ops-manager/CHANGES.md) — SDK fork change log
