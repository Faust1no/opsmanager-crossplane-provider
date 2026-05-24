# Crossplane Provider for MongoDB Ops Manager

## Overview

This provider transforms the MongoDB Ops Manager Go SDK into a Crossplane provider,
allowing you to manage Ops Manager resources declaratively via Kubernetes YAML.
It implements four managed resources targeting four specific operational goals:

1. **Project** — configure a project's LDAP group permission mappings
2. **S3Blockstore** — create and configure an S3-backed snapshot store with assignment labels
3. **S3OplogStore** — configure the shared S3-backed oplog store, appending labels per cluster
4. **BackupDaemon** — configure assignment labels on an existing backup daemon

---

## Repository Layout

```
crossplane-provider-opsmanager/
├── apis/
│   ├── v1alpha1/                    # Managed resource CRD types
│   │   ├── groupversion_info.go
│   │   ├── project_types.go
│   │   ├── s3blockstore_types.go
│   │   ├── s3oplogstore_types.go
│   │   ├── backupdaemon_types.go
│   │   └── zz_generated.deepcopy.go
│   └── v1beta1/                     # ProviderConfig CRD types
│       ├── groupversion_info.go
│       ├── providerconfig_types.go
│       └── zz_generated.deepcopy.go
├── internal/
│   ├── clients/
│   │   └── opsmanager.go            # Authenticated SDK client factory
│   └── controller/
│       ├── project/project.go       # Project reconciler
│       ├── s3blockstore/s3blockstore.go
│       ├── s3oplogstore/s3oplogstore.go
│       └── backupdaemon/backupdaemon.go
├── cmd/provider/main.go             # Binary entrypoint
└── go.mod
```

The SDK lives at `../go-client-mongodb-ops-manager` and is referenced via a
`replace` directive in `go.mod` — no published version is needed.

---

## How Crossplane Providers Work

Before diving into each file, it helps to understand the three-layer model:

```
Kubernetes CR (YAML you write)
        │
        ▼
  Crossplane Reconciler   ←  crossplane-runtime/pkg/reconciler/managed
        │   calls
        ▼
  ExternalClient          ←  your code: Observe / Create / Update / Delete
        │   calls
        ▼
  Ops Manager API         ←  via the Go SDK
```

Every reconcile loop runs the same four steps in order:

| Step        | What it does                                                                 |
|-------------|------------------------------------------------------------------------------|
| `Observe`   | Check whether the resource exists externally and whether it matches the spec |
| `Create`    | Called only when `Observe` returns `ResourceExists: false`                   |
| `Update`    | Called only when `Observe` returns `ResourceExists: true, ResourceUpToDate: false` |
| `Delete`    | Called when the CR has a deletion timestamp                                  |

The reconciler handles all the Kubernetes state machinery (conditions, status patching,
requeue logic, rate limiting). Your code only needs to talk to the external API.

---

## Authentication — `internal/clients/opsmanager.go`

The Ops Manager API uses **HTTP Digest Authentication**. Every request goes through
a challenge-response cycle: the server sends a 401 with a nonce, the client
re-sends with a hashed `Authorization` header derived from `publicKey:privateKey`.

`NewClient` wraps the SDK's HTTP client with `github.com/mongodb-forks/digest.Transport`,
which handles this handshake transparently on every request.

Credentials are never stored in any CR spec. They live in a Kubernetes Secret
referenced by the ProviderConfig:

```json
{ "publicKey": "abcdef", "privateKey": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" }
```

`ParseCredentials` deserialises this JSON into a typed `Credentials` struct.

---

## ProviderConfig — `apis/v1beta1/providerconfig_types.go`

The ProviderConfig is cluster-scoped and created once per Ops Manager instance.
All managed resources reference it by name via `spec.providerConfigRef.name`.

```yaml
apiVersion: opsmanager.crossplane.io/v1beta1
kind: ProviderConfig
metadata:
  name: default
spec:
  baseURL: "https://my-ops-manager.example.com/"
  credentials:
    source: Secret
    secretRef:
      namespace: crossplane-system
      name: opsmanager-credentials
      key: credentials   # the key inside the secret that holds the JSON
```

**`ProviderConfigUsage`** is a companion resource that Crossplane creates automatically
when a managed resource is reconciled. It records which resources are using a given
ProviderConfig so Crossplane can block deletion of a config that is still in use.

---

## Functionality 1 — Project LDAP Group Permissions

### Goal
Declaratively configure which LDAP groups map to which Ops Manager project roles,
so that LDAP-backed Ops Manager automatically grants the right permissions to
users whose LDAP group membership matches.

### CRD — `apis/v1alpha1/project_types.go`

```yaml
apiVersion: opsmanager.crossplane.io/v1alpha1
kind: Project
metadata:
  name: my-project
spec:
  providerConfigRef:
    name: default
  forProvider:
    name: "my-project-name"          # display name in Ops Manager
    orgId: "5e2211c17a3e5a48f5497de3"
    ldapGroupMappings:
      - roleName: GROUP_OWNER
        ldapGroups:
          - "cn=ops-owners,dc=example,dc=com"
      - roleName: GROUP_READ_ONLY
        ldapGroups:
          - "cn=ops-readers,dc=example,dc=com"
          - "cn=ops-viewers,dc=example,dc=com"
```

`status.atProvider.id` is populated after creation with the project ID returned
by Ops Manager. This ID is used for all subsequent API calls.

Valid `roleName` values: `GROUP_OWNER`, `GROUP_CLUSTER_MANAGER`,
`GROUP_DATA_ACCESS_ADMIN`, `GROUP_DATA_ACCESS_READ_WRITE`,
`GROUP_DATA_ACCESS_READ_ONLY`, `GROUP_READ_ONLY`, `GROUP_AUTOMATION_ADMIN`.

### SDK change — `opsmngr/projects.go`

The SDK did not have an `Update` method on `ProjectsService`. One was added:

```
PATCH /api/public/v1.0/groups/{projectId}
Body: { "ldapGroupMappings": [...] }
```

This is the only field that can be updated on an existing project via the API.
`name` and `orgId` are immutable after creation.

### Controller — `internal/controller/project/project.go`

| Method    | Behaviour |
|-----------|-----------|
| `Observe` | Calls `GetByName`. If the project exists, compares `ldapGroupMappings` against the spec. Comparison is order-insensitive within each role (groups are sorted before comparing) but role order must match. Sets `status.atProvider.id`. |
| `Create`  | Calls `Projects.Create` with `name`, `orgId`, and `ldapGroupMappings`. Stores the returned project ID in status. |
| `Update`  | Sends a PATCH with **only** `ldapGroupMappings` — no other project fields are touched. This means you can freely add, remove, or change groups for any role. |
| `Delete`  | Calls `Projects.Delete` by project ID. If the project is already gone (404), returns nil (idempotent). |

---

## Functionality 2 — S3 Blockstore with Assignment Labels

### Goal
Declaratively create an S3-backed backup store in Ops Manager and attach
`labels` to it. Backup jobs for MongoDB clusters whose assignment labels match
these labels will be routed to this store.

### CRD — `apis/v1alpha1/s3blockstore_types.go`

```yaml
apiVersion: opsmanager.crossplane.io/v1alpha1
kind: S3Blockstore
metadata:
  name: prod-s3-store
spec:
  providerConfigRef:
    name: default
  forProvider:
    id: "prod-s3-store"              # external identifier in Ops Manager
    s3BucketName: "my-backup-bucket"
    s3BucketEndpoint: "https://s3.amazonaws.com"
    s3AuthMethod: KEYS               # KEYS or IAM_ROLE
    awsAccessKey: "AKIAIOSFODNN7EXAMPLE"
    awsSecretKeySecretRef:           # secret key never lives in the CR
      namespace: crossplane-system
      name: aws-credentials
      key: secretKey
    labels:
      - "my-cluster-name"            # assignment label matching the MongoDB cluster
    assignmentEnabled: true
    pathStyleAccessEnabled: false
    acceptedTos: true
    sseEnabled: false
```

The `labels` field maps directly to `AdminBackupConfig.Labels` in the SDK.
This is the field Ops Manager uses to route backup jobs to this specific store.

**AWS Secret Key handling:** The SDK's `S3Blockstore.AWSSecretKey` is a plain
string. In this provider it is never written into the CR. Instead, `awsSecretKeySecretRef`
points to a Kubernetes Secret from which the value is fetched at reconcile time
and passed directly to the API. This means the secret key is never stored in
etcd as part of the CR.

### Controller — `internal/controller/s3blockstore/s3blockstore.go`

| Method    | Behaviour |
|-----------|-----------|
| `Observe` | Calls `S3BlockstoreConfig.Get(id)`. Compares key fields including `labels`, `s3BucketName`, `s3BucketEndpoint`, `s3AuthMethod`, `awsAccessKey`, `assignmentEnabled`, `pathStyleAccessEnabled`, `sseEnabled`, `acceptedTos`. AWS secret key is excluded from comparison — it cannot be read back from the API. Sets `status.atProvider.usedSize`. |
| `Create`  | Fetches the AWS secret key from the referenced K8s Secret, then calls `S3BlockstoreConfig.Create` with all parameters mapped to the nested SDK struct (`S3Blockstore → BackupStore → AdminBackupConfig`). |
| `Update`  | Same as Create but calls `S3BlockstoreConfig.Update(id, ...)`. Used when any tracked field drifts from the desired spec. |
| `Delete`  | Calls `S3BlockstoreConfig.Delete(id)`. Idempotent on 404. |

**`toSDKBlockstore`** maps the flat CRD parameters back to the nested SDK struct,
correctly placing `labels` inside `AdminBackupConfig`, S3-specific fields at the
top level of `S3Blockstore`, and generic blockstore fields inside `BackupStore`.

---

## Functionality 3 — S3 Oplog Store with Assignment Labels

### Goal
Declaratively configure the shared S3-backed oplog store in Ops Manager.
Unlike the snapshot blockstore (one per cluster), the oplog store is typically
**shared** — each new cluster appends its label so its continuous oplogs are
routed here. The oplog store is required for point-in-time restore.

### CRD — `apis/v1alpha1/s3oplogstore_types.go`

```yaml
apiVersion: opsmanager.crossplane.io/v1alpha1
kind: S3OplogStore
metadata:
  name: shared-oplog-store
spec:
  providerConfigRef:
    name: default
  forProvider:
    id: "shared-oplog-store"           # identifier as shown in Ops Manager
    s3BucketName: "ops-manager-oplog"
    s3BucketEndpoint: "http://minio.minio.svc.cluster.local:9000"
    s3AuthMethod: KEYS
    awsAccessKey: "minioadmin"
    awsSecretKeySecretRef:
      namespace: crossplane-system
      name: minio-credentials
      key: secretKey
    pathStyleAccessEnabled: true
    acceptedTos: true
    assignmentEnabled: true
    labels:
      - "cluster-a"    # append a new label for each cluster whose oplogs go here
      - "cluster-b"
```

The API shape is identical to `S3Blockstore` — the only difference is the
endpoint: `/admin/backup/oplog/s3Configs` instead of `/admin/backup/snapshot/s3Configs`.

### SDK change — `go-client-mongodb-ops-manager`

Two minimal additions were made to the SDK fork:

1. `opsmngr/s3_oplog_store_config.go` — new file implementing `S3OplogStoreConfigService`
   targeting `/admin/backup/oplog/s3Configs`. Reuses the existing `S3Blockstore` and
   `S3Blockstores` types — no new types were added.
2. `opsmngr/opsmngr.go` — two lines: field `S3OplogStoreConfig` on `Client` struct,
   and its initialization in `NewClient()`. This follows the exact pattern of every
   other service in the SDK.

### Controller — `internal/controller/s3oplogstore/s3oplogstore.go`

| Method    | Behaviour |
|-----------|-----------|
| `Observe` | Calls `S3OplogStoreConfig.Get(id)`. Compares the same fields as `S3Blockstore`: `labels`, `s3BucketName`, `s3BucketEndpoint`, `s3AuthMethod`, `awsAccessKey`, `assignmentEnabled`, `pathStyleAccessEnabled`, `sseEnabled`, `acceptedTos`. AWS secret key excluded from comparison. Sets `status.atProvider.usedSize`. |
| `Create`  | Fetches the AWS secret key from the referenced K8s Secret, then calls `S3OplogStoreConfig.Create`. |
| `Update`  | Calls `S3OplogStoreConfig.Update(id, ...)`. The primary use case is updating `labels` to add a new cluster. |
| `Delete`  | Calls `S3OplogStoreConfig.Delete(id)`. Idempotent on 404. |

### Adoption pattern

The oplog store is typically created once manually (or via a first `Create` reconcile),
then adopted by subsequent cluster deployments that append their label:

```yaml
# Initially:
labels: ["cluster-a"]

# When cluster-b is deployed, update to:
labels: ["cluster-a", "cluster-b"]
```

The controller detects the label drift and issues a single `Update` call to
patch the store in Ops Manager — no downtime, no recreation.

---

## Functionality 4 — BackupDaemon Assignment Labels

### Goal
Declaratively configure the `labels` on a backup daemon so that it is assigned
to handle backup jobs for specific MongoDB clusters. In your workflow, both the
S3Blockstore and the BackupDaemon that manages it need matching labels.

### CRD — `apis/v1alpha1/backupdaemon_types.go`

```yaml
apiVersion: opsmanager.crossplane.io/v1alpha1
kind: BackupDaemon
metadata:
  name: backup-agent-1
spec:
  providerConfigRef:
    name: default
  forProvider:
    machine: "backup-agent-host.example.com:27000"  # host:port identifier
    labels:
      - "my-cluster-name"    # must match the MongoDB cluster's assignment labels
    assignmentEnabled: true
```

The `machine` field is the external identifier — it matches what the backup
agent registers itself as in Ops Manager.

Only fields explicitly set in the CR are written to the daemon. All other daemon
fields (like `numWorkers`, `headDiskType`, `configured`) are left exactly as
they are in Ops Manager. This is a **merge** strategy, not a replace.

### Important: Daemons cannot be created via the API

Backup daemons auto-register themselves in Ops Manager when the backup agent
process starts. There is no API endpoint to create one. Because of this:

- If `Observe` returns 404 (daemon not registered yet), `Create` is called
- `Create` returns a clear error: _"backup daemon not found in Ops Manager;
  ensure the backup agent is running and has registered"_
- The reconciler requeues and retries, so once the agent starts and registers,
  the next reconcile will succeed and `Update` will apply the labels

### Controller — `internal/controller/backupdaemon/backupdaemon.go`

| Method    | Behaviour |
|-----------|-----------|
| `Observe` | Calls `DaemonConfig.Get(machine)`. Compares `labels`, `assignmentEnabled`, `backupJobsEnabled`, `garbageCollectionEnabled`, `resourceUsageEnabled`, `headDiskType` against spec. Sets `status.atProvider.configured`. |
| `Create`  | Returns an error telling the user to start the backup agent. The reconciler will keep retrying. |
| `Update`  | **Read-then-write**: fetches the current daemon state first, overlays only the fields set in the CR via `applyParameters`, then calls `DaemonConfig.Update`. This ensures no unmanaged fields are accidentally overwritten. |
| `Delete`  | No-op. Removing the K8s resource stops Crossplane from managing the daemon — it does not delete or modify the daemon in Ops Manager. |

**`applyParameters`** only sets a field if it was explicitly specified in the CR.
For pointer fields (`*bool`), nil means "don't touch this field". For string
fields, empty string means "don't touch". This gives you fine-grained control
over exactly which daemon properties this resource manages.

---

## End-to-End Workflow for Your Use Case

Your stated goal: when deploying a new MongoDB cluster on K8s, declaratively
configure its backup routing so that backups go to a specific S3 bucket via a
specific backup daemon.

The YAML sequence:

```
1. ProviderConfig       → authenticate to Ops Manager
2. Project              → create the project with correct LDAP roles
3. S3Blockstore         → per-cluster snapshot store labelled with the cluster name
4. S3OplogStore         → shared oplog store; append the cluster name label
5. BackupDaemon         → append the cluster name label to the backup daemon
```

Steps 3, 4, and 5 use the same label value (e.g. `"my-cluster-name"`) which
corresponds to the `assignmentLabels` set on the MongoDB cluster CR.
Ops Manager uses label matching to route: cluster → daemon → blockstore/oplog store.

For step 4 and 5, the workflow is additive: existing labels are preserved and
the new cluster label is appended, making the oplog store and daemon shared
across all clusters without disrupting existing assignments.

---

## Getting Started

### Build

```bash
cd crossplane-provider-opsmanager
go mod tidy
go build ./...
```

### Create credentials secret

```bash
kubectl create secret generic opsmanager-credentials \
  -n crossplane-system \
  --from-literal=credentials='{"publicKey":"<your-public-key>","privateKey":"<your-private-key>"}'
```

### Apply ProviderConfig

```yaml
apiVersion: opsmanager.crossplane.io/v1beta1
kind: ProviderConfig
metadata:
  name: default
spec:
  baseURL: "https://my-ops-manager.example.com/"
  credentials:
    source: Secret
    secretRef:
      namespace: crossplane-system
      name: opsmanager-credentials
      key: credentials
```

### Run locally (for development)

```bash
# Assumes KUBECONFIG is set and CRDs are installed
go run ./cmd/provider/main.go
```

---

## Known Limitations

| Area | Limitation |
|------|------------|
| Project | Only `ldapGroupMappings` can be updated. `name` and `orgId` are immutable after creation — changing them requires deleting and recreating the resource. |
| S3Blockstore | The AWS secret key cannot be read back from the Ops Manager API, so drift on that field is not detectable. It is always re-sent on every Update. |
| S3OplogStore | Same AWS secret key limitation as S3Blockstore. |
| S3OplogStore | Label order matters for drift detection — `["a","b"]` and `["b","a"]` are treated as different. Maintain a consistent order in your CR. |
| BackupDaemon | Cannot be created via the API. The backup agent must already be running and registered. The controller will keep retrying until it appears. |
| BackupDaemon | `Delete` is a no-op — it does not remove the daemon from Ops Manager. |
