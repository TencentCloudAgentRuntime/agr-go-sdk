# Migrating from `ags-go-sdk`

`agr-go-sdk` is a new product-level module. It starts with the complete managed
Sandbox journey and leaves `Client` as the reviewed expansion point for future
AGR resources. It is not a source-compatible release of `ags-go-sdk`.

Migrate one application or component at a time. Do not mix handles, generated
messages, credentials, or data-plane clients from the two modules.

## Module and package

```diff
- github.com/TencentCloudAgentRuntime/ags-go-sdk
+ github.com/TencentCloudAgentRuntime/agr-go-sdk
```

The root package name changes from `ags` to `agr`:

```go
import agr "github.com/TencentCloudAgentRuntime/agr-go-sdk"
```

The legacy module remains a separate maintenance line while existing consumers
migrate. This repository does not recreate its deep packages as compatibility
shims.

## Entry points

| Earlier API | AGR API | Notes |
| --- | --- | --- |
| `sandbox/core.Create`, `sandbox/code.Create` | `sandbox.Create` or `client.Sandboxes().Create` | Both paths return `*agr.Sandbox`. |
| `sandbox/core.Connect`, `sandbox/code.Connect` | `sandbox.Connect` or `client.Sandboxes().Connect` | Connect can Resume or extend lifetime and verifies data-plane readiness. |
| package-level `List` | `sandbox.List` or `client.Sandboxes().List` | Returns SDK-owned `SandboxPage`. |
| package-level `Kill` | `Sandbox.Delete` or `client.Sandboxes().Delete` | Delete is explicit and idempotent for NotFound. |
| `WithClient(*generated.Client)` | `agr.NewClient(...)` | Generated Cloud clients are not public configuration. |
| generated credential interfaces | `agr.CredentialProvider` | One Client binds one logical Cloud identity. |

The default shortcut reads `TENCENTCLOUD_REGION`,
`TENCENTCLOUD_SECRET_ID`, `TENCENTCLOUD_SECRET_KEY`, and optional
`TENCENTCLOUD_TOKEN`. Use separate explicit Clients for multiple identities.

## Sandbox and services

| Earlier API | AGR API |
| --- | --- |
| `SandboxId` | `ID()` |
| `GetInfo` | `Info(ctx)` |
| `SetTimeoutSeconds` | `Update(ctx, UpdateOptions{Timeout: ...})` |
| `Kill` | `Delete(ctx)` |
| `sb.Files`, `sb.Commands`, `sb.Code` | `sb.Files()`, `sb.Commands()`, `sb.Code()` |
| mutable connection config | internal generation obtained by Create, Connect, or Resume |
| `constant.EnvdPort`, `constant.CodePort` | no public replacement |
| `constant.AgentSandboxInternalEndpoint` | `WithControlPlaneEndpoint` for approved endpoint overrides |

`Close()` is local-only. Call `Delete` explicitly for an owned remote instance,
using an independent cleanup context because the operation context may already
be canceled.

Creation uses `agr.CreateOptions`, `agr.ToolRef`, `agr.MountOption`, and
`agr.SandboxAuthMode`. List, Get, Metrics, Files, Commands, PTY, Watch, and Code
use SDK-owned values. Generated Tencent Cloud, protobuf, and Connect types are
private implementation details.

Timeouts use `time.Duration` pointers. Create and Resume accept 30 seconds
through 24 hours in whole seconds; Update accepts 300 seconds through 24 hours.
Values are not clamped. A deployed policy rejection remains a structured
`*agr.Error`.

## Files, commands, and PTY

- `Read` returns `io.ReadCloser`; close complete and partial reads.
- `Write` accepts `io.Reader` and streams content without buffering the whole file.
- Rename is `Move`; file values preserve owner, group, mode, permissions, and
  optional symlink target.
- Foreground stdout and stderr are bounded independently and report truncation.
- `Start` and `Connect` return generation-bound `CommandHandle` values.
- `CommandHandle.Close` stops local observation; use `Signal` to request remote
  termination.
- PTY returns a bounded event stream and becomes usable only after its start
  barrier.

## Code contexts

`RunCodeConfig.ContextId` becomes `RunCodeOptions.Context`:

```go
managed, err := sb.Code().CreateContext(ctx, agr.CreateCodeContextOptions{})
result, err := sb.Code().Run(
    ctx,
    source,
    agr.RunCodeOptions{Context: managed},
    callbacks,
)
```

A managed `CodeContext` belongs to one Sandbox generation. Cross-Sandbox use
and use after Pause or Resume fail locally. To ask the service to validate an
existing ID explicitly:

```go
external, err := agr.NewExternalCodeContextRef(existingContextID)
result, err := sb.Code().Run(
    ctx,
    source,
    agr.RunCodeOptions{Context: external},
    callbacks,
)
```

An external reference does not prove existence, ownership, or validity after a
lifecycle transition. The SDK does not create a replacement context to imitate
recovery.

## Errors

Replace string matching with `errors.As(err, &sdkError)` and branch on
`sdkError.Code` and `sdkError.Reason`. A Create failure after service acceptance
can contain an `InstanceID`; a mutation with unknown outcome is not permission
to retry.

## Consumer migration gate

Before removing `ags-go-sdk` from a consumer:

1. replace all legacy imports, including deep `connection`, `constant`, `pb`,
   `sandbox/*`, and `tool/*` paths;
2. compile with the consumer's declared Go toolchain;
3. run its deterministic tests and the AGR SDK consumer fixture;
4. perform separately authorized Cloud E2E when the consumer changes lifecycle
   or data-plane behavior;
5. confirm explicit cleanup and absence of secret-bearing logs.

`ags-cli` currently imports legacy deep packages. Its migration is a separate
downstream change and is not implied by the creation of this module.
