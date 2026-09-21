# Changelog

All notable changes are documented here. The project follows semantic
versioning after 1.0; pre-1.0 versions can include documented
source-incompatible changes.

## Unreleased

### Added

- A new `github.com/TencentCloudAgentRuntime/agr-go-sdk` module with root
  package `agr`.
- A product-level `agr.Client` and canonical `agr.Sandbox` resource.
- Environment-backed `sandbox.Create`, `Connect`, `Get`, and `List` shortcuts.
- Sandbox lifecycle, Pause, Resume, WaitFor, Update, Delete, Cloud Monitor
  Metrics, PTY, Watch, Commands List/Connect, streaming files, and
  generation-bound Code contexts.
- SDK-owned errors and models, bounded streams and aggregates, protocol and
  source provenance, and offline verification gates.
- A migration guide for users of `ags-go-sdk`.
- Stable Tool create/get/list/update/delete/wait operations and image pre-cache create/get.
- Cursor-based Sandbox listing with opaque `NextToken` and optional exact total counts.
- Reproducible stable and preview control-plane snapshots with explicit preview-field selection,
  stable-superset verification, and semantic-version release-channel checks.

### Changed

- The module path and root package name are AGR-owned. The Cloud protocol keeps
  its deployed AGS Action, endpoint, version, and metric namespace names.
- Cloud and Monitor orchestration lives in `internal/controlplane`, generation
  and bounded handle ownership in `internal/runtime`, data-plane transport in
  `internal/dataplane`, and generated bindings in `internal/gen`.
- The control-plane contract is a reproducible allowlisted snapshot of the
  pinned CLI effective API.
- The control-plane manifest records stable/preview source channel, resource ownership,
  maturity, transport route, request/response types, and per-Action object closures.
- Tencent Cloud AGS and common modules are updated to 1.3.179.

### Removed

- Legacy `ags-go-sdk` deep packages and public generated types are not copied
  into the new module. See
  [MIGRATING_FROM_AGS_GO_SDK.md](MIGRATING_FROM_AGS_GO_SDK.md).
