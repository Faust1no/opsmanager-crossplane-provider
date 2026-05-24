# Testing the Provider Locally

The provider runs **out-of-cluster** using your local kubeconfig.
No Docker image is needed. The binary talks to the kind cluster via kubeconfig
and to Ops Manager via localhost:32001.

---

## Step 1 — Build

```bash
cd crossplane-provider-opsmanager
go mod tidy
go build ./...
```

---

## Step 2 — Install the CRDs

```bash
kubectl apply -f package/crds/
```

Verify they are registered:
```bash
kubectl get crds | grep opsmanager
# Expected:
# backupdaemons.opsmanager.crossplane.io
# projects.opsmanager.crossplane.io
# providerconfigs.opsmanager.crossplane.io
# providerconfigusages.opsmanager.crossplane.io
# s3blockstores.opsmanager.crossplane.io
```

---

## Step 3 — Create an Ops Manager API Key

1. Open Ops Manager at http://localhost:32001
2. Log in as `admin@example.com` / `OpsManager@SecurePass123!`
3. Go to: top-right menu → **Organization** → **Access Manager** → **API Keys**
4. Click **Create API Key**, give it `Organization Owner` permission
5. Copy the **Public Key** and **Private Key**

Also grab your **Organization ID**:
- Top-left org menu → **Settings** → copy the **Organization ID**

---

## Step 4 — Fill in the examples

Edit `examples/providerconfig.yaml`:
- Replace `YOUR_PUBLIC_KEY` and `YOUR_PRIVATE_KEY`

Edit `examples/project.yaml`:
- Replace `YOUR_ORG_ID`
- Adjust LDAP group DNs to match your glauth config

Edit `examples/backupdaemon.yaml`:
- Replace `YOUR_DAEMON_HOST:27018` with the actual machine string.
  Find it in Ops Manager UI → **Admin** → **Backup** → **Daemons** tab
  after Ops Manager has reached Running phase.

---

## Step 5 — Apply ProviderConfig

```bash
kubectl apply -f examples/providerconfig.yaml
```

---

## Step 6 — Run the provider

```bash
go run ./cmd/provider/main.go
```

You will see log output like:
```
{"level":"debug","ts":"...","logger":"provider-opsmanager","msg":"Starting manager"}
{"level":"info","ts":"...","msg":"Starting workers","controller":"project..."}
```

Leave this running in a terminal. It watches for CRs and reconciles them.

---

## Step 7 — Test Project + LDAP

```bash
kubectl apply -f examples/project.yaml

# Watch status
kubectl get project test-project -w

# Once READY=True, inspect the result
kubectl describe project test-project
```

Verify in Ops Manager UI → **Projects** — the project should appear with
the LDAP group mappings configured.

To test updating LDAP groups, edit `examples/project.yaml` (add/remove a group),
then re-apply. The controller detects the drift and PATCHes only `ldapGroupMappings`.

---

## Step 8 — Test S3 Blockstore

```bash
kubectl apply -f examples/s3blockstore.yaml

kubectl get s3blockstore minio-blockstore -w
kubectl describe s3blockstore minio-blockstore
```

Verify in Ops Manager UI → **Admin** → **Backup** → **S3 Snapshot Stores** —
the blockstore should appear with the `my-cluster` label.

---

## Step 9 — Test S3 Oplog Store

The oplog store is shared across clusters. Create it once, then append labels for each
new cluster. First create the MinIO bucket for oplogs if it does not exist:

```bash
kubectl run -it --rm mc --image=quay.io/minio/mc:latest --restart=Never -- \
  /bin/sh -c "mc alias set local http://minio.minio.svc.cluster.local:9000 minioadmin minioadmin123 && mc mb --ignore-existing local/ops-manager-oplog"
```

Then apply the CR:

```bash
kubectl apply -f examples/s3oplogstore.yaml

kubectl get s3oplogstore shared-oplog-store -w
kubectl describe s3oplogstore shared-oplog-store
```

Verify in Ops Manager UI → **Admin** → **Backup** → **Oplog Stores** —
the store should appear with the `test-rs` label.

To add a second cluster later, edit the CR and append its label:
```yaml
labels:
  - "test-rs"
  - "cluster-b"
```
Re-apply and the controller will PATCH the store in Ops Manager.

---

## Step 10 — Test BackupDaemon

The backup daemon only appears in Ops Manager after the backup daemon pod has
started and registered itself. Once it has:

```bash
kubectl apply -f examples/backupdaemon.yaml

kubectl get backupdaemon ops-manager-daemon -w
kubectl describe backupdaemon ops-manager-daemon
```

If the daemon hasn't registered yet, the CR will show:
```
Message: backup daemon not found in Ops Manager; ensure the backup agent is running
```
It will keep retrying automatically.

Verify in Ops Manager UI → **Admin** → **Backup** → **Daemons** —
the daemon should show the `my-cluster` label.

---

## Useful debug commands

```bash
# See all managed resources and their status
kubectl get project,s3blockstore,s3oplogstore,backupdaemon

# Check conditions on a resource
kubectl get project test-project -o jsonpath='{.status.conditions}' | jq

# Check which resources are using the ProviderConfig
kubectl get providerconfigusages

# Force a re-reconcile by adding an annotation
kubectl annotate project test-project reconcile=now --overwrite
```

---

## Known Issues & TODOs

### S3OplogStore and S3Blockstore label order
Label drift detection compares slices positionally. `["a","b"]` and `["b","a"]` are treated
as different, triggering a spurious Update. Keep labels in a consistent order in your CRs.

---

### S3Blockstore / S3OplogStore Delete blocked by Ops Manager (409 BACKUP_CANNOT_REMOVE_S3_STORE_CONFIG)

**Problem:** When a blockstore has `assignmentEnabled: true`, Ops Manager refuses to delete it
with a 409 even if no MongoDB cluster or snapshots are bound to it.

**Root cause:** The controller's `Delete()` method calls `DELETE` on the API directly without
first disabling assignment. Ops Manager requires `assignmentEnabled: false` before it will
accept the delete.

**Why the Delete method does NOT auto-patch assignmentEnabled:**
Automatically setting `assignmentEnabled: false` inside `Delete()` would be a side effect
outside the declarative model — the controller would be mutating external state beyond what
the spec describes. This is considered an anti-pattern in Crossplane providers and Kubernetes
operators in general. The same pattern is followed by other providers (e.g. the AWS provider
returns an error if an S3 bucket is not empty before deletion rather than emptying it).

**Correct approach:** Set `spec.forProvider.assignmentEnabled: false` in the CR before deleting.
The `Update()` reconcile loop will apply this to Ops Manager. Once `assignmentEnabled` is false
in Ops Manager, deleting the CR will succeed.

**The controller will surface:** `BACKUP_CANNOT_REMOVE_S3_STORE_CONFIG` (409) as the error
on the CR's condition until `assignmentEnabled` is set to false.

---

## Teardown

```bash
kubectl delete -f examples/
kubectl delete -f package/crds/
```
