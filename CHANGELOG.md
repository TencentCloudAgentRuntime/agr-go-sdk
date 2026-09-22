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
- Stable Deployment create/get/list/update/delete/wait operations backed by six official typed
  Cloud Actions, plus managed direct HTTP, SSE, and WebSocket data-plane connections.
- Lazy, coalesced Deployment token refresh, connection affinity handling, connection-scoped
  cancellation, deterministic loopback coverage, a cookbook journey, and opt-in real Cloud E2E.
- Preview `Volume` and `VolumeTemplate` managers with create/get/list/update/delete operations,
  plus Tool mounts by Volume ID/name or VolumeTemplate reuse key and a persistent-storage
  cookbook journey.
- Preview Volume and VolumeTemplate mutations by exact account-local name; Tool template mounts
  may omit `ReuseKey` when the service-side template does not require one.
- Preview `SessionSpace` and `Session` managers covering 12 reviewed Actions, bounded structured
  state and Events, explicit three-part Session identity, a cookbook journey, and Shanghai Cloud
  E2E coverage.
- Preview `Registry` and Registry-scoped Record managers covering 17 reviewed metadata Actions,
  Version decisions and labels, bounded descriptor JSON, audit logs, explicit Preview/Sync, and a
  cookbook journey. Skill package URL issuance and package transfer remain deferred.

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
- The stable control-plane allowlist remains 19 typed Actions. Preview adds 37 reviewed Volume,
  VolumeTemplate, SessionSpace, Session, Event, and Registry Actions through an internal
  `CommonClient` allowlist; there is no public raw call or typed-to-raw fallback.

### Removed

- Legacy `ags-go-sdk` deep packages and public generated types are not copied
  into the new module. See
  [MIGRATING_FROM_AGS_GO_SDK.md](MIGRATING_FROM_AGS_GO_SDK.md).
