# Architecture

[中文](ARCHITECTURE-zh.md)

The SDK keeps one cohesive product model while separating private Cloud and runtime
implementation by ownership. Applications import only the root `agr` package and,
optionally, the `sandbox` shortcut package.

## Public model

```text
Client
  |-- SandboxManager -> Sandbox
  |                      |-- Files
  |                      |-- Commands
  |                      |-- PTY
  |                      |-- Code
  |                      `-- Metrics
  |-- ToolManager -> ToolInfo / ToolMutationResult
  |-- PreCacheImageManager -> PreCacheImageInfo
  |-- DeploymentManager -> DeploymentInfo
  |                       `-> DeploymentConnection -> HTTP / SSE / WebSocket
  |-- VolumeManager -> VolumeInfo
  |-- VolumeTemplateManager -> VolumeTemplateInfo
  |-- SessionSpaceManager -> SessionSpaceInfo
  |-- SessionManager -> SessionInfo / EventInfo
  `-- RegistryManager -> RegistryInfo
                         `-- RegistryRecordManager -> Record / Version / AuditLog
```

`Client` binds one Tencent Cloud identity, region, endpoints, and request policy.
Create and Connect return the canonical `*agr.Sandbox`. The `sandbox` package is a
small environment-backed convenience layer over the same Client and Sandbox types;
it does not contain another implementation.

Tool and pre-cache managers are account-level, control-plane-only services. Their returned
descriptions, mutation receipts, and task status are immutable SDK-owned values rather than
connected handles: they have no local generation and no `Close`. A mutation never triggers an
implicit Describe, and deleting a Tool never cascades to Sandboxes.

Preview Volume and VolumeTemplate managers are also account-level, control-plane-only services.
They return immutable SDK-owned values. A Tool mount selects exactly one of a direct storage
source, an existing Volume reference, or a VolumeTemplate reference. Deleting a Sandbox or Tool
does not imply deletion of standalone storage; template reclaim policy is server-owned and callers
still explicitly clean retained derived Volumes.

Preview SessionSpace and Session managers are account-level, control-plane-only services. A
Session has an explicit Space, User, and Session identity; no component is inferred globally.
State and Event JSON are SDK-owned bounded values. Sessions have no Sandbox generation,
CodeContext lifetime, Deployment affinity, or local `Close` operation. Remote cleanup is always
explicit and ordered Session before SessionSpace.

The Preview Registry manager is an account-level control-plane service. A Record manager binds
one explicit Registry ID, and Version selection is explicit and mutually exclusive by Version ID
or label. Descriptor, source-configuration, preview, and audit JSON are bounded SDK-owned values.
Remote `Preview` is read-only; `Sync` is a caller-requested mutation. A5a deliberately excludes
package URLs and package transport, so pre-signed URLs cannot escape through the public model.

Deployment control operations also return immutable snapshots. `DeploymentConnection` is the
only stateful Deployment object: it binds one Deployment, region, domain, and Tool port. It owns
lazy token refresh, affinity discovery/rotation, active HTTP bodies, and WebSockets. Its `Close`
cancels local operations and never performs Cloud deletion. The public API exposes neither the
short-lived token nor a configurable request authority.

`Client` is the stable expansion point for AGR resources. A new resource service is
added only with a public Cloud or data-plane contract, explicit ownership and lifecycle,
stable errors, deterministic tests, a cookbook journey, and release documentation.
Speculative resource names and empty service interfaces are not part of the public API.

## Dependency direction

```text
agr / sandbox (public facades)
        |
        +--> internal/controlplane --> official Tencent Cloud SDK
        |
        +--> internal/runtime --> internal/dataplane --> internal/gen
        |
        `--> internal/dataplane/deployment
                    |
                    `--> internal/model
```

- The root package owns public options, validation, stable errors, and conversion
  between public values and private DTOs.
- `internal/controlplane` owns Cloud clients, signing integration, typed Action mapping,
  lifecycle polling, Metrics requests, instance-token acquisition, and construction
  of a fresh runtime generation.
- Preview Volume, Session, and Registry Actions use the official Tencent Cloud `common.CommonClient` only through a
  compile-time internal allowlist because the pinned generated AGS models do not contain them.
  Existing typed Actions never fall back to this path after failure, and no raw call is public.
- `internal/runtime` owns generation validity, request lifetimes, handles, stream
  pumps, bounded queues, output aggregation, and local invalidation.
- `internal/dataplane` owns immutable endpoints and instance access material,
  authentication headers, HTTP/Connect calls, protocol parsing, and start barriers.
- `internal/dataplane/deployment` owns the direct Deployment HTTPS and WSS transport, coalesced
  token refresh, affinity sequencing, and connection-scoped cancellation. Its WebSocket library
  type remains private.
- `internal/gen` contains generated protocol bindings. It may be consumed only by
  the data-plane implementation and protocol fixtures.
- `internal/model` contains the smallest wire-independent DTO and error vocabulary
  needed across private layers. It never aliases public or generated types.

Internal packages never import the root `agr` package. Root production code never
imports Connect, generated bindings, or the Tencent Cloud generated SDK.

## Lifecycle ownership

Each connected Sandbox owns exactly one current data-plane generation. Readers,
watches, command handles, PTYs, Code executions, and managed Code contexts belong to
the generation that created them.

- Pause invalidates the current generation before the Cloud mutation is submitted.
- Resume obtains new instance access material and installs a new generation only
  after the data plane is ready.
- Close synchronously invalidates local work and never deletes the remote instance.
- Delete is the explicit remote cleanup operation.
- Caller cancellation stops local observation; it does not prove that an accepted
  remote side effect was undone.

Normal stream close, generation invalidation, and explicit remote process signals are
separate operations. Moving implementation across layers must not merge those meanings.

A Deployment connection has a separate, simpler lifecycle. `Connect` first verifies that the
Deployment is ACTIVE, but tokens are acquired only on the first data request. The SDK refreshes
them before the two-hour safety window and coalesces concurrent refreshes. If refresh fails, a
still-valid token may be used. A 401 invalidates the token without replaying the request. Affinity
IDs are learned from the configured response header; discovery and rotation are serialized so a
late response cannot restore stale affinity. Connection close cancels HTTP/SSE/WebSocket work.

## Adding a capability

1. Define SDK-owned public inputs and results only when a user-facing contract is needed.
2. Add the smallest private DTO to its owning internal package; use `internal/model`
   only when more than one private layer needs the value.
3. Add semantic data-plane operations instead of exposing clients, endpoints, tokens,
   generated messages, or generic request builders.
4. Put lifecycle and bounded asynchronous behavior in `internal/runtime`, not in a
   public handle wrapper.
5. Add deterministic loopback tests for mappings, cancellation, bounds, errors, and
   side-effect semantics. Real Cloud tests remain explicitly opt-in.
6. Run `make verify`. The repository verifier enforces the dependency direction and
   the approved public-root responsibilities.

Public API changes require prior discussion, an API snapshot review, migration notes,
and a changelog entry. An internal refactor must keep the public API snapshot unchanged.

The allowlist and generated control-plane snapshots are provenance, not a second public API. The
v2 manifests record their pinned CLI revision and source channel, effective-contract hashes, and
each Action's resource, maturity, route, request/response types, and referenced object closure.
Stable reads the base API directly. Preview delegates patch composition to the CLI, filters out
unselected patched fields, and must remain a structural superset of stable. Preview is delivered
through semantic-version prereleases; it is not selected at runtime or with Go build tags.
Stable and preview public Go API snapshots are stored separately. The reviewed registry selects
which snapshot represents the current source, while release checks select one from the tag.

## Verification

The default verification is offline and includes formatting, tests, vet, race tests,
public consumer compilation, API snapshot checks, generated-code drift checks,
repository architecture checks, and secret scanning. Optional Cloud validation and
its required environment are documented in [test/README.md](test/README.md).

See [CONTRIBUTING.md](CONTRIBUTING.md) for the contribution workflow.
