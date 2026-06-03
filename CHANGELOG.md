# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [Unreleased]

## [1.1.0] - 2026-06-03
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
