# Provider Conventions & AI Integration Guide

This document captures every convention used in this provider so that new
managed resources can be implemented consistently — whether by a human or by an
AI assistant. Read this before starting any implementation work.

---

## Repository layout

```
apis/
  v1alpha1/
    <kind>_types.go          ← CRD type definition for each managed resource
    zz_generated.deepcopy.go ← hand-maintained DeepCopy methods (no codegen)
    groupversion_info.go     ← scheme registration
  v1beta1/
    providerconfig_types.go  ← ProviderConfig and ProviderConfigUsage
cmd/
  provider/
    main.go                  ← entry point; registers all controllers
internal/
  clients/
    opsmanager.go            ← Credentials struct + NewClient factory
  controller/
    <kind>/
      <kind>.go              ← one controller per managed resource
package/
  crds/
    opsmanager.crossplane.io_<plural>.yaml  ← CRD manifests applied to the cluster
examples/
  <kind>.yaml                ← example CR for testing
```

---

## Part 1 — Adding a new managed resource

There are five files to touch for every new kind.

### 1. Type definition — `apis/v1alpha1/<kind>_types.go`

Follow this exact structure:

```go
package v1alpha1

// <Kind>GroupKind and <Kind>GroupVersionKind are used by the controller Setup function.
var <Kind>GroupKind        = schema.GroupKind{Group: Group, Kind: "<Kind>"}
var <Kind>GroupVersionKind = SchemeGroupVersion.WithKind("<Kind>")

// <Kind>Parameters holds all writable fields. Map these 1:1 from the SDK struct.
// Use *bool for optional booleans (never plain bool) so nil means "not set".
// Use +kubebuilder:default=<val> for fields that have a meaningful default.
type <Kind>Parameters struct {
    // ID is the identifier used to look up the resource in Ops Manager.
    // +kubebuilder:validation:MinLength=1
    ID string `json:"id"`

    // ... other fields ...
}

// <Kind>Observation holds read-only fields returned by the API (atProvider).
// Only include fields the API returns that are useful to surface to the user.
type <Kind>Observation struct {
    // e.g. UsedSize int64 `json:"usedSize,omitempty"`
}

// <Kind>Spec and <Kind>Status follow this exact pattern — no variations.
type <Kind>Spec struct {
    xpv1.ResourceSpec `json:",inline"`
    ForProvider       <Kind>Parameters `json:"forProvider"`
}

type <Kind>Status struct {
    xpv1.ConditionedStatus `json:",inline"`
    AtProvider             <Kind>Observation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:resource:scope=Namespaced,categories=crossplane  // or scope=Cluster for admin-level resources (e.g. BackupDaemon)
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"

type <Kind> struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec   <Kind>Spec   `json:"spec"`
    Status <Kind>Status `json:"status,omitempty"`
}

// resource.Managed interface — copy this block verbatim and replace <Kind>.
// Do NOT add any logic here; these are pure forwarding methods.
func (mg *<Kind>) GetCondition(ct xpv1.ConditionType) xpv1.Condition {
    return mg.Status.GetCondition(ct)
}
func (mg *<Kind>) SetConditions(c ...xpv1.Condition) { mg.Status.SetConditions(c...) }
func (mg *<Kind>) GetDeletionPolicy() xpv1.DeletionPolicy { return mg.Spec.DeletionPolicy }
func (mg *<Kind>) SetDeletionPolicy(r xpv1.DeletionPolicy) { mg.Spec.DeletionPolicy = r }
func (mg *<Kind>) GetManagementPolicies() xpv1.ManagementPolicies { return mg.Spec.ManagementPolicies }
func (mg *<Kind>) SetManagementPolicies(r xpv1.ManagementPolicies) { mg.Spec.ManagementPolicies = r }
func (mg *<Kind>) GetProviderReference() *xpv1.Reference         { return mg.Spec.ProviderReference }
func (mg *<Kind>) SetProviderReference(r *xpv1.Reference)        { mg.Spec.ProviderReference = r }
func (mg *<Kind>) GetProviderConfigReference() *xpv1.Reference   { return mg.Spec.ProviderConfigReference }
func (mg *<Kind>) SetProviderConfigReference(r *xpv1.Reference)  { mg.Spec.ProviderConfigReference = r }
func (mg *<Kind>) GetPublishConnectionDetailsTo() *xpv1.PublishConnectionDetailsTo {
    return mg.Spec.PublishConnectionDetailsTo
}
func (mg *<Kind>) SetPublishConnectionDetailsTo(r *xpv1.PublishConnectionDetailsTo) {
    mg.Spec.PublishConnectionDetailsTo = r
}
func (mg *<Kind>) GetWriteConnectionSecretToReference() *xpv1.SecretReference {
    return mg.Spec.WriteConnectionSecretToReference
}
func (mg *<Kind>) SetWriteConnectionSecretToReference(r *xpv1.SecretReference) {
    mg.Spec.WriteConnectionSecretToReference = r
}

// +kubebuilder:object:root=true
type <Kind>List struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []<Kind> `json:"items"`
}

func init() {
    SchemeBuilder.Register(&<Kind>{}, &<Kind>List{})
}
```

**Key rules:**
- `*bool` everywhere for optional booleans — never `bool`. The API distinguishes
  "false" from "not set".
- `+kubebuilder:default` only when the Ops Manager API has a required field with
  a clear safe default (e.g. `s3AuthMethod: KEYS`, `s3MaxConnections: 50`).
- Secrets (e.g. AWS secret key) are always referenced via `*xpv1.SecretKeySelector`,
  never stored inline in the CR spec.

---

### 2. DeepCopy methods — `apis/v1alpha1/zz_generated.deepcopy.go`

`controller-gen` is not wired up. DeepCopy methods are maintained by hand.
Add a section following the exact pattern of existing kinds. The file has
section comments like `// ---- S3Blockstore ----` to keep it navigable.

```go
// ---- <Kind> ----

func (in *<Kind>) DeepCopyInto(out *<Kind>) {
    *out = *in
    out.TypeMeta = in.TypeMeta
    in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
    in.Spec.DeepCopyInto(&out.Spec)
    in.Status.DeepCopyInto(&out.Status)
}

func (in *<Kind>) DeepCopy() *<Kind> {
    if in == nil { return nil }
    out := new(<Kind>)
    in.DeepCopyInto(out)
    return out
}

func (in *<Kind>) DeepCopyObject() runtime.Object {
    if c := in.DeepCopy(); c != nil { return c }
    return nil
}

func (in *<Kind>List) DeepCopyInto(out *<Kind>List) {
    *out = *in
    out.TypeMeta = in.TypeMeta
    in.ListMeta.DeepCopyInto(&out.ListMeta)
    if in.Items != nil {
        in, out := &in.Items, &out.Items
        *out = make([]<Kind>, len(*in))
        for i := range *in { (*in)[i].DeepCopyInto(&(*out)[i]) }
    }
}

func (in *<Kind>List) DeepCopy() *<Kind>List {
    if in == nil { return nil }
    out := new(<Kind>List)
    in.DeepCopyInto(out)
    return out
}

// Spec, Status, Parameters, Observation — add DeepCopyInto for any field
// that contains a slice, map, or pointer. Scalar-only structs just need:
func (in *<Kind>Spec) DeepCopyInto(out *<Kind>Spec) {
    *out = *in
    in.ResourceSpec.DeepCopyInto(&out.ResourceSpec)
    in.ForProvider.DeepCopyInto(&out.ForProvider)
}
// If Parameters contains []string labels:
func (in *<Kind>Parameters) DeepCopyInto(out *<Kind>Parameters) {
    *out = *in
    if in.Labels != nil {
        in, out := &in.Labels, &out.Labels
        *out = make([]string, len(*in))
        copy(*out, *in)
    }
    // repeat for every slice/map/pointer field
}
```

---

### 3. Controller — `internal/controller/<kind>/<kind>.go`

```go
package <kind>

// Error constants — always the first thing in the file after imports.
// Follow the "cannot <verb> <noun>" pattern.
const (
    errNot<Kind>         = "managed resource is not a <Kind>"
    errGetProviderConfig = "cannot get ProviderConfig"
    errCreateClient      = "cannot create Ops Manager client"
    errTrackUsage        = "cannot track ProviderConfig usage"
    errGet<Kind>         = "cannot get <kind> from Ops Manager"
    errCreate<Kind>      = "cannot create <kind> in Ops Manager"
    errUpdate<Kind>      = "cannot update <kind> in Ops Manager"
    errDelete<Kind>      = "cannot delete <kind> from Ops Manager"
)
```

**`Setup`** — identical across all controllers, only the type names change:

```go
func Setup(mgr ctrl.Manager, o controller.Options) error {
    name := managed.ControllerName(v1alpha1.<Kind>GroupKind.Kind)
    r := managed.NewReconciler(mgr,
        resource.ManagedKind(v1alpha1.<Kind>GroupVersionKind),
        managed.WithExternalConnecter(&connector{
            kube:  mgr.GetClient(),
            usage: resource.NewProviderConfigUsageTracker(mgr.GetClient(), &v1beta1.ProviderConfigUsage{}),
        }),
        managed.WithLogger(o.Logger.WithValues("controller", name)),
        managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
    )
    return ctrl.NewControllerManagedBy(mgr).
        Named(name).
        WithOptions(o.ForControllerRuntime()).
        For(&v1alpha1.<Kind>{}).
        Complete(r)
}
```

**`connector.Connect`** — identical across all controllers:

```go
func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
    cr, ok := mg.(*v1alpha1.<Kind>)
    if !ok {
        return nil, errors.New(errNot<Kind>)
    }
    if err := c.usage.Track(ctx, mg); err != nil {
        return nil, errors.Wrap(err, errTrackUsage)
    }
    pc := &v1beta1.ProviderConfig{}
    if err := c.kube.Get(ctx, types.NamespacedName{Name: cr.GetProviderConfigReference().Name}, pc); err != nil {
        return nil, errors.Wrap(err, errGetProviderConfig)
    }
    creds, err := clients.GetCredentials(ctx, c.kube, pc)
    if err != nil {
        return nil, err
    }
    opsClient, err := clients.NewClient(pc.Spec.BaseURL, creds)
    if err != nil {
        return nil, errors.Wrap(err, errCreateClient)
    }
    return &external{service: opsClient.<ServiceField>, kube: c.kube}, nil
}
```

**`Observe`** — the decision-making method:

```go
func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
    cr := mg.(*v1alpha1.<Kind>)

    // If Delete is a no-op (resource intentionally left in Ops Manager on CR
    // deletion), return ResourceExists=false here so the finalizer clears immediately.
    if meta.WasDeleted(cr) {
        return managed.ExternalObservation{ResourceExists: false}, nil
    }

    observed, _, err := e.service.Get(ctx, cr.Spec.ForProvider.ID)
    if isNotFound(err) {
        return managed.ExternalObservation{ResourceExists: false}, nil
    }
    if err != nil {
        return managed.ExternalObservation{}, errors.Wrap(err, errGet<Kind>)
    }

    // Write read-only API state back into atProvider.
    cr.Status.AtProvider.UsedSize = observed.UsedSize
    meta.SetExternalName(cr, cr.Spec.ForProvider.ID)
    cr.SetConditions(xpv1.Available())

    return managed.ExternalObservation{
        ResourceExists:   true,
        ResourceUpToDate: isUpToDate(cr.Spec.ForProvider, observed),
    }, nil
}
```

**`isUpToDate`** — compare spec against observed state field by field:

```go
func isUpToDate(p v1alpha1.<Kind>Parameters, o *opsmngr.<SDKType>) bool {
    if p.SomeField != o.SomeField { return false }
    if !stringSlicesEqual(p.Labels, o.Labels) { return false }
    if !boolPtrEqual(p.AssignmentEnabled, o.AssignmentEnabled) { return false }
    return true
}
```

Rules:
- AWS secret key is **always excluded** from `isUpToDate` — it cannot be read
  back from the Ops Manager API.
- Label comparison is positional (`stringSlicesEqual`) — document this in
  TESTING.md known issues if relevant.

**Helper functions** — copy these verbatim into every controller that needs them.
Do not deduplicate into a shared package; keeping them local avoids coupling.

`clients.GetCredentials` is the one exception: credential fetching is shared in
`internal/clients/opsmanager.go` because all controllers connect to the same Ops
Manager instance with the same API key pair.

```go
func stringSlicesEqual(a, b []string) bool {
    if len(a) != len(b) { return false }
    for i := range a { if a[i] != b[i] { return false } }
    return true
}

func boolPtrEqual(a, b *bool) bool {
    if a == nil && b == nil { return true }
    if a == nil || b == nil { return false }
    return *a == *b
}

func isNotFound(err error) bool {
    if err == nil { return false }
    var e *opsmngr.ErrorResponse
    return stderrors.As(err, &e) && e.Response != nil && e.Response.StatusCode == http.StatusNotFound
}
```

---

### 4. Register in `cmd/provider/main.go`

Add one import and one `Setup` call following the existing order:

```go
import (
    // existing imports ...
    "github.com/crossplane-contrib/provider-opsmanager/internal/controller/<kind>"
)

// In main():
if err := <kind>.Setup(mgr, o); err != nil {
    log.Debug("Cannot setup <Kind> controller", "error", err)
    os.Exit(1)
}
```

---

### 5. CRD manifest — `package/crds/opsmanager.crossplane.io_<plural>.yaml`

`controller-gen` is not wired up. Copy the closest existing CRD YAML and
replace kind/plural names and the `spec.versions[0].schema.openAPIV3Schema`
properties to match the new type's fields. The boilerplate around
`status.conditions`, `spec.providerConfigRef`, `spec.deletionPolicy`, and
`spec.managementPolicies` is identical for every resource — do not change it.

---

## Part 2 — Adoption pattern

When the external resource already exists in Ops Manager and the CR is being
applied for the first time (to bring an existing resource under management),
`Observe` will find it by ID and return `ResourceExists: true`. No special
handling is needed — the controller adopts it automatically.

If the spec matches the observed state exactly, `isUpToDate` returns true and
no API call is made. If there is drift, `Update` is called to reconcile it.

---

## Part 3 — Delete semantics

Two patterns are used depending on the resource:

**Pattern A — hard delete** (S3Blockstore, S3OplogStore, Project):
`Delete` calls the Ops Manager API. If the API returns 404, treat as success
(idempotent). The managed reconciler removes the finalizer after `Delete`
returns nil.

**Pattern B — no-op delete** (BackupDaemon):
The external resource should remain in Ops Manager when the CR is deleted
(the resource lifecycle is owned by something else — in this case, the pod).
In this pattern:
1. `Observe` must return `ResourceExists: false` when `meta.WasDeleted(cr)` is
   true. This lets the managed reconciler skip calling `Delete` and go straight
   to removing the finalizer.
2. `Delete` returns nil immediately.

If you forget the `meta.WasDeleted` check in `Observe` for Pattern B, the
controller will loop forever: `Observe` returns `ResourceExists: true` →
`Delete` returns nil → requeue → repeat, and the finalizer is never removed.

---

## Part 4 — Prompt template for new managed resources

Use this prompt when asking an AI assistant to implement a new managed resource.
Fill in the bracketed sections before sending.

```
We are working in the crossplane-provider-opsmanager provider at
/home/crossplane-faust/crossplane-provider-opsmanager.

Read CONVENTIONS.md in full before writing any code — every decision in there
must be followed exactly. Also read the SDK fork conventions at
../go-client-mongodb-ops-manager/CHANGES.md.

Implement a new managed resource for the following Ops Manager API endpoint:

  Resource kind:    <Kind>                  (e.g. OplogStore)
  CRD plural:       <plural>                (e.g. oplogstores)
  SDK service:      opsClient.<Field>       (e.g. opsClient.S3OplogStoreConfig)
  API path:         <path>                  (e.g. /api/public/v1.0/admin/backup/oplog/s3Configs)
  HTTP verbs:       GET, POST, PUT, DELETE  (list which apply)

Fields to expose in ForProvider (from the API docs or SDK struct):
  - <fieldName> <type> — <description>
  - ...

Read-only fields to surface in AtProvider:
  - <fieldName> <type> — <description>

Delete behaviour:
  [ ] Hard delete — call DELETE on the API and let Crossplane remove the finalizer.
  [ ] No-op delete — leave the resource in Ops Manager; add meta.WasDeleted check
      in Observe and return nil from Delete.

Does this require a new SDK service in the fork? [ ] Yes  [ ] No
If yes, describe the endpoint and response shape so CHANGES.md can be updated.

Files to create or modify:
  1. apis/v1alpha1/<kind>_types.go         (new)
  2. apis/v1alpha1/zz_generated.deepcopy.go (modify — add section)
  3. internal/controller/<kind>/<kind>.go  (new)
  4. cmd/provider/main.go                  (modify — add import + Setup call)
  5. package/crds/opsmanager.crossplane.io_<plural>.yaml (new)
  6. examples/<kind>.yaml                  (new)
  7. CONVENTIONS.md                        (modify if a new pattern was introduced)
  8. ../go-client-mongodb-ops-manager/CHANGES.md (modify if SDK was touched)
```
