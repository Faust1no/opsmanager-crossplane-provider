# Adoption chart templates

Minimum-spec Helm-style templates for adopting an existing Ops Manager resource.
See the **Adopting existing Ops Manager resources** section of the [README](README.md)
for the required-field rules these templates follow.

All templates assume a `ClusterProviderConfig` named `default` already exists in
the cluster. This is the recommended setup — it works for every kind and matches
the typical one-Ops-Manager-per-cluster topology. `BackupDaemon` and
`S3OplogStore` are cluster-scoped and can *only* use `ClusterProviderConfig`.

For a multi-Ops-Manager setup (different OMs per namespace), the two namespaced
kinds (`OpsManagerProject`, `S3Blockstore`) can instead reference a
`kind: ProviderConfig` in the same namespace.

## `OpsManagerProject`

Adoption key: `spec.forProvider.name` must match the project's display name in
Ops Manager. The OM-generated project ID is discovered automatically.

```yaml
apiVersion: opsmanager.crossplane.io/v1alpha1
kind: OpsManagerProject
metadata:
  name: {{ .Values.project.name }}
  namespace: {{ .Release.Namespace }}
spec:
  providerConfigRef:
    kind: ClusterProviderConfig
    name: default
  forProvider:
    name:  {{ .Values.project.displayName }}    # adoption key
    orgId: {{ .Values.project.orgID }}
    # Optional — leave nil to inherit existing API mappings:
    # ldapGroupMappings:
    #   - roleName: GROUP_OWNER
    #     ldapGroups: ["cn=om-owners,ou=groups,dc=example,dc=com"]
```

## `S3Blockstore`

Adoption key: `spec.forProvider.id` must match the existing store's `id` in
Ops Manager. The `crossplane.io/external-name` annotation is set automatically
by the controller after the first observe; you do not need to template it.

```yaml
apiVersion: opsmanager.crossplane.io/v1alpha1
kind: S3Blockstore
metadata:
  name: {{ .Values.blockstore.id }}
  namespace: {{ .Release.Namespace }}
spec:
  providerConfigRef:
    kind: ClusterProviderConfig
    name: default
  forProvider:
    id:                      {{ .Values.blockstore.id }}        # adoption key
    s3BucketEndpoint:        {{ .Values.blockstore.endpoint }}
    s3BucketName:            {{ .Values.blockstore.bucket }}
    s3AuthMethod:            KEYS
    awsAccessKey:            {{ .Values.blockstore.accessKey }}
    awsSecretKeySecretRef:
      namespace: {{ .Release.Namespace }}
      name:      {{ .Values.blockstore.secretName }}
      key:       secretKey
    # Optional fields you may want to set in a chart:
    # pathStyleAccessEnabled: true        # for MinIO and most non-AWS S3
    # assignmentEnabled:     true
    # labels: ["rs-1"]
    # uri: mongodb://...                  # only if the store has a colocated mongod
```

## `S3OplogStore`

Cluster-scoped (one oplog config per `id` globally in Ops Manager). Adoption
key and required `forProvider` fields are identical to `S3Blockstore`; only the
OM endpoint differs (oplog vs. snapshot). Only `ClusterProviderConfig` is
allowed in `providerConfigRef`.

```yaml
apiVersion: opsmanager.crossplane.io/v1alpha1
kind: S3OplogStore
metadata:
  name: {{ .Values.oplogstore.id }}
spec:
  providerConfigRef:
    kind: ClusterProviderConfig
    name: default
  forProvider:
    id:                      {{ .Values.oplogstore.id }}        # adoption key
    s3BucketEndpoint:        {{ .Values.oplogstore.endpoint }}
    s3BucketName:            {{ .Values.oplogstore.bucket }}
    s3AuthMethod:            KEYS
    awsAccessKey:            {{ .Values.oplogstore.accessKey }}
    awsSecretKeySecretRef:
      namespace: {{ .Release.Namespace }}
      name:      {{ .Values.oplogstore.secretName }}
      key:       secretKey
```

## `BackupDaemon`

Adoption key: `spec.forProvider.machine` must equal the daemon's hostname as
shown in Ops Manager (Admin → Backup → Daemons). `Create` is unsupported — the
daemon must already exist in Ops Manager or the CR stays `Ready=False`.

```yaml
apiVersion: opsmanager.crossplane.io/v1alpha1
kind: BackupDaemon
metadata:
  name: {{ .Values.daemon.shortName }}
spec:
  providerConfigRef:
    kind: ClusterProviderConfig
    name: default
  forProvider:
    machine: {{ .Values.daemon.machine }}    # adoption key
    # Optional:
    # labels: ["rs-1"]
    # assignmentEnabled: true
```
