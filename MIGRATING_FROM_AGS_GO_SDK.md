# Migrating from `ags-go-sdk`

[中文](MIGRATING_FROM_AGS_GO_SDK-zh.md)

This guide compares the published `ags-go-sdk` **v0.1.5** API (its `main` branch
at `351c2e5`) with `agr-go-sdk` `main`. The earlier SDK's unreleased
`codex/sdk-v0-convergence` branch already changed many of these APIs; it is
not the migration baseline. Pin a published `agr-go-sdk` tag when migrating a
production consumer.

`agr-go-sdk` is a new product-level module. It starts with the complete managed
Sandbox journey and adds other reviewed AGR resources through `Client`. It is
not a source-compatible release of `ags-go-sdk`.

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

## Breaking changes at a glance

| v0.1.5 usage | AGR replacement or consequence |
| --- | --- |
| `sandbox/core`, `sandbox/code`, `tool/*`, `connection`, `constant`, or `pb/*` imports | Use the root `agr` package and its `sandbox` shortcuts. The deep packages and public generated types have no source-compatible replacement. |
| `Create(ctx, toolName, options...)` | Pass `CreateOptions{Tool: ToolRef{Name: toolName}}`; choose `ToolRef{ID: id}` for an exact Tool ID. Options are fields on a value, not the old per-call option functions. |
| `WithClient(*ags.Client)` or `common.CredentialIface` | Build an `agr.Client` with `WithCredential`, `WithTemporaryCredential`, or `WithCredentialProvider`. A generated Cloud client cannot be injected. |
| `WithDataPlaneDomain("example.com")` for Sandbox | Use `WithSandboxDomain("ap-guangzhou.example.com")`: the new value includes the region and applies to Sandbox port hosts and managed runtime connections. `WithDataPlaneDomain` now configures **Deployment only**. |
| `WithScheme("http")`, mutable `ConnectionConfig`, or `tool/*`.New with a supplied instance Token | There is no direct Token-only data-plane entry or HTTP-scheme switch. Managed connections use Cloud credentials to acquire access material and use HTTPS. A consumer requiring direct Token access cannot migrate this path unchanged. |
| `sb.GetHost(port) string` | Handle `(string, error)`; invalid ports are rejected. |
| `sb.Kill(ctx)` or package-level `Kill` | Use `sb.Delete(ctx)` or `client.Sandboxes().Delete(ctx, id)`. `Close()` releases local handles only. |
| `sb.Files`, `sb.Commands`, `sb.Code` fields | Call `sb.Files()`, `sb.Commands()`, `sb.Code()`; do not retain services across a closed Sandbox. |
| `List(ctx)` returning `[]*ags.SandboxInstance` | `List(ctx, SandboxListOptions{})` returns one `SandboxPage` with SDK-owned values; use `ListPage`/`Iterate` for cursor traversal. |
| `RunCodeConfig.ContextId` | Use a managed `CodeContext` or an explicit `NewExternalCodeContextRef(id)`; the latter relies on server validation. |

The old SDK allowed direct construction of Files, Commands, and Code clients
from `connection.Config{Domain, AccessToken, Headers, Proxy, Scheme}`. The new
SDK deliberately has no equivalent for arbitrary instance Tokens, custom
data-plane headers, or a manually selected data-plane URL. `WithHTTPClient`
can supply trusted HTTP transport settings, but it does not restore those
public connection controls. Inventory such uses before changing imports.

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

For the common v0.1.5 `sandbox/code.Create(ctx, toolName, ...)` path, the new
creation call is:

```go
func run(ctx context.Context, toolName string) (err error) {
    lifetime := 5 * time.Minute // Preserve the old SDK default.
    sb, err := sandbox.Create(ctx, sandbox.CreateOptions{
        Tool:    sandbox.ToolRef{Name: toolName},
        Timeout: &lifetime,
    })
    if err != nil {
        return err
    }
    defer sb.Close()
    defer func() {
        cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()
        err = errors.Join(err, sb.Delete(cleanup))
    }()
    // Use sb.Files(), sb.Commands(), or sb.Code() here.
    return nil
}
```

The old SDK defaulted creation to five minutes. A nil AGR `Timeout` leaves the
choice to the service. The old `SandboxConfig.Timeout` string and pointer-valued
mount fields become `*time.Duration` and `[]agr.MountOption` values. Set
`AuthMode` with an `agr.SandboxAuthMode` constant. See the
[default creation cookbook](examples/cookbook/01_default/main.go) for cleanup.

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

`Connect` also changes behavior: v0.1.5 obtained an instance Token and returned
without probing the data plane. AGR reads the instance state, may Resume or
extend its lifetime, and waits for data-plane readiness. If the caller only
needs metadata, use `client.Sandboxes().Get(ctx, id)`. Do not connect merely to
display an existing instance.

The old Sandbox domain option appended the Cloud region to a base domain. AGR
expects the complete suffix:

```go
client, err := agr.NewClient(
    agr.WithRegion("ap-guangzhou"),
    agr.WithSandboxDomain("ap-guangzhou.example.com"),
)
// Later: host, err := sb.GetHost(8080)
```

`WithControlPlaneEndpoint` is separate from this Sandbox domain. A custom
domain requires working DNS, TLS, and routing; `GetHost` only computes a name.

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
- Rename is `Move`. `MakeDir`, `Move`, and `Write` return `FileInfo` values rather
  than the old bool, error-only, and `WriteInfo` results. `GetInfo` is `Stat`;
  file values preserve owner, group, mode, permissions, and optional symlink
  target. Check code that assumes the old file-type string values.
- The old command client ran `"echo hello"` through `/bin/bash -l -c` when
  `ProcessConfig.Args` was empty. AGR sends the command and arguments directly.
  To preserve the old shell behavior, call `sb.Commands().Run(ctx, "/bin/bash",
  agr.CommandOptions{Args: []string{"-lc", "echo hello"}})`.
- Foreground stdout and stderr are bounded independently and report truncation.
- `Run` takes value options and returns a value `CommandResult`. There is no
  `OnOutputConfig` parameter; use `Start` and its `Events()` channel for live
  output. A nonzero exit code is in the result rather than a Go transport error.
- `Start` and `Connect` return generation-bound `CommandHandle` values. Use
  `PID()` instead of the old `Pid` field, and `Write(ctx, data)` instead of
  `SendInput(ctx, pid, data)`.
- `CommandHandle.Close` replaces observation-only `Disconnect`; use
  `Signal(ctx, agr.SignalKILL)` to replace `Kill` and request remote termination.
  Closing the handle does not kill the process.
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

`RunCode` is now `sb.Code().Run(ctx, source, options, callbacks)`. The old
`Execution.Logs.Stdout/Stderr` fields are `CodeExecution.Stdout/Stderr`; rich
results are `CodeResult` values with Go initialisms such as `HTML`, `JSON`, and
`PNG`. Retained output and callbacks are bounded. If a Tool does not provide
the Code service, `Code().Run` still requires a suitable Tool; merely creating
a Sandbox does not add that service.

## Errors

Replace string matching with `errors.As(err, &sdkError)` and branch on
`sdkError.Code` and `sdkError.Reason`. A Create failure after service acceptance
can contain an `InstanceID`; a mutation with unknown outcome is not permission
to retry.

## Consumer migration gate

Before removing `ags-go-sdk` from a consumer:

1. inventory direct instance-Token clients, custom schemes/headers/proxies,
   and direct protobuf or Tencent Cloud model use; decide how each caller will
   use the managed API before removing the old package;
2. replace legacy imports, including deep `connection`, `constant`, `pb`,
   `sandbox/*`, and `tool/*` paths, then select a published AGR version;
3. compile with the consumer's declared Go toolchain and run its deterministic
   tests and the AGR SDK consumer fixture;
4. test command shell behavior, output limits, explicit reader/handle closing,
   Connect side effects, timeout defaults, and remote deletion;
5. perform separately authorized Cloud E2E when lifecycle or data-plane
   behavior changes, and confirm cleanup and absence of secret-bearing logs.

`ags-cli` currently imports legacy deep packages. Its migration is a separate
downstream change and is not implied by the creation of this module.
