# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [Unreleased]
### Added
- Namespace-scoped `ProviderConfig` (and `ProviderConfigUsage`) alongside the existing
  cluster-scoped `ClusterProviderConfig`. Namespaced managed resources can now pick a
  per-namespace Ops Manager via `providerConfigRef.kind: ProviderConfig`.
- Unified `clients.Resolve` and `clients.UsageTracker` helpers in `internal/clients`
  so each managed resource controller's `Connect` is two lines (`Track` + `Resolve`)
  instead of a per-kind switch.
- Provider `Makefile` at the repo root with developer inner-loop targets:
  `generate`, `build`, `image`, `xpkg`, `load`, `install`, `redeploy`, `logs`,
  `status`, `lint`, `test`, `vendor`, `clean`.
- Optional log rotation: `--log-file`, `--log-file-max-size`, `--log-file-max-backups`,
  `--log-file-max-age` CLI flags (off by default). Implemented via `lumberjack`.
- `DEVELOPMENT.md` section 2 documenting the provider Makefile and the dev loop.

### Changed
- `cmd/provider/main.go` rewritten with `kingpin` flags (`--debug`, `--sync-interval`,
  `--poll-interval`, `--leader-election`, `--max-reconcile-rate`). Startup failures
  now log at Info level instead of Debug (previous Debug-then-exit was silent at default
  verbosity).
- All managed-resource controllers (`project`, `s3blockstore`, `s3oplogstore`,
  `backupdaemon`) now use `clients.NewUsageTracker` and `clients.Resolve`,
  removing the inlined switch-on-kind boilerplate from each `Connect`.
- `internal/controller/config/config.go` now wires both `ClusterProviderConfig` and
  `ProviderConfig` reconcilers via `providerconfig.NewReconciler`.

### Fixed
- `S3Blockstore` and `S3OplogStore` labels can now be fully deleted via YAML.
  `isUpToDate` now always compares labels once the `opsmanager.crossplane.io/labels-adopted`
  annotation is set, treating a nil spec as "user wants no labels" and triggering an Update
  to clear them in the API.

## [1.1.1] - 2026-06-04
### Fixed
- `S3Blockstore` and `S3OplogStore` labels can now be deleted via YAML. Labels are adopted
  from the API exactly once (on first Observe) and the annotation
  `opsmanager.crossplane.io/labels-adopted` is set; after that the spec YAML is the source
  of truth and the API never overwrites a user-removed label.

## [1.1.0] - 2026-06-03
### Added
- CI workflow with golangci-lint and CHANGELOG enforcement.
### Fixed
- `lateInitBlockstore` and `lateInitOplogStore` now populate `s3BucketEndpoint` and
  `s3AuthMethod` from the API response, preventing spurious Updates when adopting an
  existing store with a minimal spec (only `id` and `s3BucketName` specified).
- `LoadFactor` and `MaxCapacityGB` (`*int64` fields) are now late-initialised correctly
  using a typed pointer helper instead of the `*bool` helper.
- `OpsManagerProject` creation no longer fails with `BILLING_UNSUPPORTED`; the project
  is created with `withDefaultAlertsSettings: false` to skip Atlas-only billing alerts.

## [1.0.0] - 2026-05-01
### Changed
- Upgraded `crossplane-runtime` from `v0.20.1` to `v1.16.0`, natively fixing the
  `RetryingCriticalAnnotationUpdater` namespace bug that caused `external-create-pending`
  annotations to never persist on namespaced resources, leaving creates permanently stuck.
- Removed the custom `NamespacedCriticalAnnotationUpdater` workaround (no longer needed).
- Removed `GetProviderReference` / `SetProviderReference` methods from all managed resource
  types — these were removed from the `resource.Managed` interface in crossplane-runtime v1.x.

## [0.9.0] - 2026-04-01
### Added
- `S3OplogStore` managed resource targeting the Ops Manager oplog S3 store endpoint
  (`/admin/backup/oplog/s3Configs`). Reuses the same SDK types as `S3Blockstore`.
- Custom `NamespacedCriticalAnnotationUpdater` to work around the namespace bug in
  `crossplane-runtime` v0.20.1.

## [0.8.0] - 2026-03-01
### Added
- Initial release with `OpsManagerProject`, `S3Blockstore`, and `BackupDaemon` controllers.
- `S3Blockstore` adopt-on-create: if the blockstore already exists in Ops Manager, the
  controller adopts it instead of failing.
- `BackupDaemon` read-then-write update strategy to avoid overwriting unmanaged daemon fields.
- HTTP Digest authentication via `github.com/mongodb-forks/digest`.
