# Repository guidance

This repository is the public Go SDK for Tencent Cloud Agent Runtime (AGR).
Keep every tracked file suitable for a public source repository.

## Product boundary

- Keep the module path `github.com/TencentCloudAgentRuntime/agr-go-sdk` and the
  root package name `agr`.
- `Client` is the product-level root for credentials, region, endpoints,
  transport policy, and first-class resources.
- `Sandbox` is the first implemented resource. Its Files, Commands, Code, PTY,
  Watch, Metrics, and lifecycle APIs remain generation-bound capabilities of
  that resource.
- The `sandbox` package contains only environment-backed shortcuts that return
  the same `*agr.Sandbox` type as `Client.Sandboxes()`.
- Do not add placeholder services for Volume, Deployment, Session, Registry,
  or another future resource. A resource enters the public API only with
  a reviewed public service contract, lifecycle and error semantics, a
  cookbook journey, deterministic tests, and release documentation.

The Cloud service and endpoint can retain the AGS protocol name. Do not rename
wire Actions, API versions, endpoints, metric namespaces, or copied contract
paths merely to match the AGR SDK brand.

## Migration and compatibility

This repository was bootstrapped from the reviewed implementation in
`TencentCloudAgentRuntime/ags-go-sdk`. Provenance is recorded in
`SOURCE_ATTRIBUTION.md`; do not remove or weaken it.

`agr-go-sdk` is a new module, not an in-place compatible release of
`ags-go-sdk`. Migration behavior belongs in
`MIGRATING_FROM_AGS_GO_SDK.md`. Do not add legacy packages such as
`connection`, `constant`, `pb`, `sandbox/core`, `sandbox/code`, or `tool/*` to
simulate source compatibility. The old module remains a separate maintenance
line until its consumers migrate.

The SDK is pre-1.0. Public changes still require an API diff, tests, an
`Unreleased` changelog entry, and migration notes when users must change code.
Preview capabilities use semantic-version prereleases. Do not add a runtime
channel option, environment toggle, request header, or Go build tag. Stable
tags must not contain preview Actions or fields. Preserve the stable public API
snapshot while developing preview capabilities; update only the preview snapshot.

## Architecture

Keep the root `agr` package as the cohesive public facade. Put Cloud and
Monitor orchestration in `internal/controlplane`, Sandbox generation and handle
ownership in `internal/runtime`, runtime authentication and transports in
`internal/dataplane`, shared private DTOs in `internal/model`, and generated
bindings in `internal/gen`. Follow [ARCHITECTURE.md](ARCHITECTURE.md).

- Prefer the official Tencent Cloud Go SDK for Cloud Actions. An internal
  generic transport may cover only a reviewed Action listed in
  `contracts/control-plane.json`; never expose arbitrary Action calls.
- Public signatures must not expose Tencent Cloud generated models, protobuf
  messages, Connect types, endpoints, headers, or secret-bearing connection
  objects.
- Internal packages must not import the public root package.
- Do not implement or fork TC3 request signing.

## Implementation rules

- Support Go 1.22 with `GOTOOLCHAIN=local`.
- Every remote operation accepts `context.Context`; cancellation never proves
  that a remote side effect was undone.
- Do not automatically retry mutations, command execution, uploads, or streams.
- Copy caller-owned maps, slices, and pointed-to options before asynchronous use.
- Keep readers, event queues, callbacks, command output, Code events, and
  aggregation bounded.
- Bind data-plane requests and handles to one Sandbox generation. Pause,
  Resume, and Close must not revive an old generation.
- Use `*agr.Error` for stable public error categories. Formatted errors must not
  contain credentials, authorization headers, instance access material,
  response bodies, or causes.
- Use `apply_patch` for source edits. Preserve unrelated contributor changes.

## Verification

Run `make verify` before proposing a change. Default verification must remain
offline. Tests that need loopback servers are unit tests; tests that contact
Tencent Cloud require explicit environment opt-in and must independently clean
up every created resource. The E2E contract is [test/README.md](test/README.md).

When proto files change, update `contracts/proto.json`, regenerate with pinned
tools, and prove that the generation drift check fails on a temporary change
and passes after restoration. Do not edit generated files by hand.

## Documentation and security

Keep English and Chinese README and contribution guides aligned. Examples must
compile, use placeholders, call `Close`, and show explicit remote cleanup.
Never commit credentials, tokens, real resource identifiers, cloud outputs,
private endpoints, local filesystem paths, progress ledgers, approval records,
or unpublished product plans.

Maintain `LICENSE`, `NOTICE`, `SOURCE_ATTRIBUTION.md`, `THIRD_PARTY.md`,
`SECURITY.md`, `CODE_OF_CONDUCT.md`, and contribution documentation as release
artifacts. Any copied source or dependency change must update its provenance,
hashes, license inventory, and generated-code relationship in the same change.
