# Tencent Cloud Agent Runtime Go SDK

[中文](README-zh.md)

The native Go SDK for Tencent Cloud Agent Runtime (AGR). `Client` is the
product-level entry point for credentials, regions, transport policy, and
first-class resources. The current release provides managed `Sandbox`
lifecycles and data-plane operations, stable Deployment control and managed
HTTP/SSE/WebSocket connections, Tool management, and image pre-cache tasks.
Preview prereleases additionally expose Volume and VolumeTemplate management, persistent Tool
mounts, SessionSpace/Session/Event control APIs, Registry/Record/Version APIs, and managed Skill
package transfer.

This is a new Go module, not an in-place release of `ags-go-sdk`. Existing
users should read [Migrating from ags-go-sdk](MIGRATING_FROM_AGS_GO_SDK.md).
Future resources are added only after their public service contract and user
journey are reviewed; this repository does not publish placeholder APIs.

## Requirements

- Go 1.22 or later
- Tencent Cloud Agent Sandbox access
- A Sandbox Tool in the selected region
- Network access to Tencent Cloud APIs and the Sandbox data plane

## Install

```bash
go get github.com/TencentCloudAgentRuntime/agr-go-sdk
```

## Authentication

Control-plane and Metrics requests use Tencent Cloud credentials:

```bash
export TENCENTCLOUD_REGION=ap-guangzhou
export TENCENTCLOUD_SECRET_ID=your-secret-id
export TENCENTCLOUD_SECRET_KEY=your-secret-key
# Optional for temporary Tencent Cloud credentials:
export TENCENTCLOUD_TOKEN=your-session-token
```

The SDK obtains Sandbox instance and Deployment access material through the control plane when
needed. That material is internal: it is not accepted or returned by the public API and is never
placed in a URL.

## Quick start

The `sandbox` package is the shortest path for one environment-backed Cloud identity:

```go
package main

import (
	"context"
	"log"
	"time"

	agr "github.com/TencentCloudAgentRuntime/agr-go-sdk"
	"github.com/TencentCloudAgentRuntime/agr-go-sdk/sandbox"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	lifetime := 10 * time.Minute
	sb, err := sandbox.Create(ctx, sandbox.CreateOptions{
		Tool:    sandbox.ToolRef{ID: "your-tool-id"},
		Timeout: &lifetime,
		Env:     map[string]string{"APP_ENV": "demo"},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer sb.Close() // local resources only

	result, err := sb.Commands().Run(ctx, "sh", agr.CommandOptions{
		Args: []string{"-lc", "printf 'hello from sandbox'"},
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("exit=%d stdout=%s", result.ExitCode, result.Stdout)

	cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	if err := sb.Delete(cleanup); err != nil {
		log.Fatal(err)
	}
}
```

## Explicit Client and multiple identities

Construct a Client when configuration must be explicit or a process uses more than one Cloud
identity. One Client binds one logical identity; create a separate Client for each account.

```go
client, err := agr.NewClient(
	agr.WithRegion("ap-guangzhou"),
	agr.WithCredentialProvider(provider),
)
if err != nil {
	return err
}

sb, err := client.Sandboxes().Connect(ctx, sandboxID)
if err != nil {
	return err
}
defer sb.Close()
```

The shortcut and explicit Client paths return the same `*agr.Sandbox` type and share the same
implementation.

For a Sandbox data-plane hostname suffix other than the regional default, set
`agr.WithSandboxDomain("ap-guangzhou.example.com")` on that Client. The value is a complete
hostname suffix, without a scheme or port. `sb.GetHost(8080)` then returns
`8080-<sandbox-id>.ap-guangzhou.example.com`, and SDK file, command, and Code connections use
the same suffix. `WithControlPlaneEndpoint` configures the separate Cloud API address;
`WithDataPlaneDomain` retains its existing Deployment-only meaning. The domain must have working
DNS, TLS, and routing; `GetHost` does not check whether the Tool exposes port 8080. See the
[explicit Client cookbook](examples/cookbook/02_explicit_client/main.go).

For a small UTF-8 file, `sb.Files().ReadText(ctx, path, agr.ReadOptions{}, maxBytes)` closes the
stream and rejects content beyond the positive `maxBytes` limit, which may be at most 16 MiB.
Use streaming `Read` for larger or binary files. To traverse Sandbox cursor pages lazily, call
`iterator := client.Sandboxes().Iterate(options)`, then `iterator.Next(ctx)` until `io.EOF` and
`defer iterator.Close()`. The iterator retains at most one page, supports cancellation, and does
not promise a snapshot of the changing remote list. `ListPage` remains available when the caller
needs explicit cursor control. See the [file cookbook](examples/cookbook/01_default/main.go) and
[list cookbook](examples/cookbook/05_cursor_list/main.go).

Tool and pre-cache operations are available only from the explicit Client because they are
account-level control-plane resources:

```go
tools, err := client.Tools().List(ctx, agr.ToolListOptions{Limit: 20})
if err != nil {
	return err
}

task, err := client.PreCacheImages().Create(ctx, agr.CreatePreCacheImageOptions{
	Image:        "ccr.ccs.tencentyun.com/example/runtime:v1",
	RegistryType: agr.ImageRegistryPersonal,
})
```

Deployments are also explicit Client resources. Control operations return immutable snapshots;
`Connect` returns a local connection for one Tool port:

```go
deployment, err := client.Deployments().WaitFor(ctx, deploymentID, agr.DeploymentActive)
if err != nil {
	return err
}
connection, err := client.Deployments().Connect(ctx, deployment.ID, agr.DeploymentConnectOptions{Port: 8080})
if err != nil {
	return err
}
defer connection.Close() // local HTTP/SSE/WebSocket operations only

response, err := connection.Do(ctx, agr.DeploymentRequest{Method: http.MethodGet, Path: "/health"})
```

`Do` supports ordinary HTTP and incrementally consumed SSE responses. `DialWebSocket` returns an
SDK-owned, context-aware WebSocket handle. The access token, target authority, and affinity state
remain private; requests cannot select another host. See the complete
[Deployment cookbook](examples/cookbook/08_deployment/main.go).

`Client` is intentionally broader than Sandbox. Additional first-class resources are exposed as
peer services only after their server contract, lifecycle, errors, tests, and cookbook journey
are available. This repository does not publish placeholder services.

Preview releases provide `client.Volumes()` and `client.VolumeTemplates()`. A Tool mount chooses
one source: `StorageSource`, `ToolVolumeRef`, or `ToolVolumeTemplateRef`. Template reuse keys,
including `${metadata.session_id}`, are sent unchanged. Deleting a Sandbox or Tool does not
implicitly delete a standalone Volume; a retained template-derived Volume also requires explicit
cleanup. See the [persistent Volume cookbook](examples/cookbook/09_persistent_volume/main.go).

Preview releases also provide `client.SessionSpaces()` and `client.Sessions()`. Every Session is
identified by `SessionRef{SpaceID, UserID, SessionID}`; the SDK never fills an implicit Space or
User. JSON state, deltas, event metadata, extensions, and function payloads are validated as
bounded JSON objects. Event append mutations are not retried. See the
[Session and Events cookbook](examples/cookbook/10_session_events/main.go).

Preview releases also provide `client.Registries()`. Records are always scoped through
`client.Registries().Records(registryID)`; there is no implicit default Registry. Version IDs and
labels use an explicit mutually exclusive selector. Descriptor and audit JSON is copied and
bounded to 1 MiB. `Preview` is read-only, while `Sync` is an explicit mutation and is never
retried. Registry sources include CUSTOM, MCP/A2A/AG-UI manual or URL-import content, inline
Skill text, and `TAR_PACKAGE`. Skill archives use bounded streaming upload/download with local
SHA-256 verification; short-lived pre-signed URLs remain internal and downloads are never
extracted by the SDK. See the [Registry metadata cookbook](examples/cookbook/11_registry_records/main.go)
and [Skill package cookbook](examples/cookbook/12_registry_skill_package/main.go).

## Capabilities

| Area | API |
| --- | --- |
| Sandbox control | `Create`, `Connect`, `Get`, offset `List`, cursor `ListPage`/`Iterate`, `Pause`, `Resume`, `WaitFor`, `Update`, `Delete` |
| Files | streaming `Read`/`Write`, bounded UTF-8 `ReadText`, `Stat`, `Exists`, `List`, `MakeDir`, `Move`, `Remove`, `Watch` |
| Commands | `Run`, `Start`, `List`, `Connect`, bounded output, input and signals |
| Code | `Run`, managed contexts, explicit external context references, bounded callbacks |
| PTY | start barrier, input, resize, bounded event stream |
| Metrics | ten typed Cloud Monitor series with `Start`, `End`, period, and partial success |
| Tools | `Create`, exact `Get`, offset `List`, `Update`, `WaitFor`, idempotent `Delete` |
| Image pre-cache | `Create` and `Get` by image, digest, and registry type |
| Deployment control | `Create`, `Get`, `List`, `Update`, `WaitFor`, `Delete`, `WaitDeleted` |
| Deployment data plane | direct managed HTTP/SSE `Do`, context-aware WebSocket, token refresh, affinity |
| Preview Volume | `Create`, exact `Get`, offset `List`, `Update`, `Delete` |
| Preview VolumeTemplate | `Create`, exact `Get`, offset `List`, `Update`, `Delete`, Tool reuse-key mounts |
| Preview SessionSpace | `Create`, exact `Get`, offset `List`, `Update`, `Delete` |
| Preview Session and Events | Session `Create`, exact `Get`, offset `List`, `Update`, `Delete`; Event `AppendEvent`, `ListEvents` |
| Preview Registry | Registry and Record CRUD/List, Version decisions and labels, audit logs, explicit `Preview`/`Sync`, bounded Skill package upload/download |

## Lifecycle rules

- `Sandbox.Close()` releases local readers, streams, and observations. It does not delete the
  remote instance.
- `Sandbox.Delete(ctx)` explicitly stops the remote instance and treats NotFound as success.
- Pause invalidates the current data-plane generation before submitting the control request.
  Resume obtains fresh connection material; old readers, handles, watches, PTYs, executions,
  and managed code contexts remain invalid.
- Context cancellation stops local observation. It does not prove that a remote mutation or
  process was undone.
- Mutating operations are not retried automatically.
- Tool mutation receipts confirm service acceptance; they are not synthesized `ToolInfo`
  snapshots. `ToolInfo` and pre-cache results are values and do not need `Close`.
- Tool deletion never cascades to Sandboxes. Delete dependent Sandboxes first and then delete only
  Tool IDs created by your workflow.
- Pre-cache creation does not mean the image is ready. Poll `Get` with the returned complete
  image/digest/registry reference. Stable inputs accept `enterprise` and `personal`; `custom` is
  intentionally not promised without a verified service contract.
- `DeploymentConnection.Close()` cancels its active HTTP/SSE/WebSocket work and does not delete
  the Deployment. Deletion is asynchronous; call `Delete` and `WaitDeleted` explicitly.
- Deployment access tokens are acquired lazily, refreshed internally, redacted, and never exposed
  as public input or output. A 401 is returned to the caller and invalidates the cached token; the
  SDK does not replay the request.
- Registry and Record values are account-level control-plane snapshots and do not need `Close`.
  Delete Records before their Registry. Registry mutations, `Sync`, and package uploads are not
  retried. A failed download may leave a prefix in its destination; use a temporary file when
  atomic replacement is required.

Create and Resume accept whole-second lifetimes from 30 seconds through 24 hours. Update uses
a 300-second minimum. Connect reads state first: a paused instance follows Resume rules and a
running instance follows Update rules. The SDK sends valid values unchanged; a deployed Cloud
policy may still reject 30-299 seconds, in which case the original structured service error is
returned without clamping or an automatic retry.

## Errors

Use `errors.As` to inspect `*agr.Error`. Its stable fields include `Code`, `Reason`,
`Operation`, `RequestID`, `Retryable`, `InstanceID`, `ResourceType`, `ResourceID`, and optional
mutation evidence. The formatted error omits credentials, authorization headers, resource IDs,
causes, and response bodies.

## Development

```bash
make verify
```

Default tests are offline. Real Cloud tests require explicit opt-in and are documented in
[CONTRIBUTING.md](CONTRIBUTING.md). Protocol and control-plane provenance is recorded under
[`contracts/`](contracts/). The verification suite also compiles the public consumer fixtures.
The v2 control-plane manifest pins the CLI revision and source channel, hashes the effective
contract, and records each Action's resource, maturity, typed route, request/response types, and
referenced object closure.

Preview capabilities, when present, use explicit semantic-version prereleases such as
`v0.3.0-preview.1`. The module path and public resource names remain unchanged. The SDK has no
runtime preview switch, environment toggle, request-level channel, or Go build tag. Rows labeled
Preview are present only in preview prereleases; all other rows are stable. A preview version is
published only after its contract, public API, tests, and real-service evidence are reviewed.

See the [Architecture](ARCHITECTURE.md), [Cookbook](examples/cookbook/README.md),
[Security Policy](SECURITY.md), and [Contributing Guide](CONTRIBUTING.md).

## License

Apache License 2.0. See [LICENSE](LICENSE), [NOTICE](NOTICE), and
[SOURCE_ATTRIBUTION.md](SOURCE_ATTRIBUTION.md).
