# Tencent Cloud Agent Runtime Go SDK

[中文](README-zh.md)

The native Go SDK for Tencent Cloud Agent Runtime (AGR). `Client` is the
product-level entry point for credentials, regions, transport policy, and
first-class resources. The initial release implements the complete managed
`Sandbox` journey: lifecycle, streaming files, commands, PTY, file watch, code
execution contexts, and ten Cloud Monitor metrics.

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

The SDK obtains Sandbox instance access material through the control plane after Create,
Connect, or Resume. That material is internal: it is not accepted or returned by the public
API and is never placed in a URL.

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

`Client` is intentionally broader than Sandbox. Later first-class resources
such as Volume or Deployment can be exposed as peer services without turning
`Sandbox` into a universal client. No such service is present until its server
contract, lifecycle, errors, tests, and cookbook journey are available.

## Capabilities

| Area | API |
| --- | --- |
| Control | `Create`, `Connect`, `Get`, `List`, `Pause`, `Resume`, `WaitFor`, `Update`, `Delete` |
| Files | streaming `Read`/`Write`, `Stat`, `Exists`, `List`, `MakeDir`, `Move`, `Remove`, `Watch` |
| Commands | `Run`, `Start`, `List`, `Connect`, bounded output, input and signals |
| Code | `Run`, managed contexts, explicit external context references, bounded callbacks |
| PTY | start barrier, input, resize, bounded event stream |
| Metrics | ten typed Cloud Monitor series with `Start`, `End`, period, and partial success |

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

Create and Resume accept whole-second lifetimes from 30 seconds through 24 hours. Update uses
a 300-second minimum. Connect reads state first: a paused instance follows Resume rules and a
running instance follows Update rules. The SDK sends valid values unchanged; a deployed Cloud
policy may still reject 30-299 seconds, in which case the original structured service error is
returned without clamping or an automatic retry.

## Errors

Use `errors.As` to inspect `*agr.Error`. Its stable fields include `Code`, `Reason`,
`Operation`, `RequestID`, `Retryable`, `InstanceID`, and optional mutation evidence. The
formatted error omits credentials, authorization headers, instance IDs, causes, and response
bodies.

## Development

```bash
make verify
```

Default tests are offline. Real Cloud tests require explicit opt-in and are documented in
[CONTRIBUTING.md](CONTRIBUTING.md). Protocol and control-plane provenance is recorded under
[`contracts/`](contracts/). The verification suite also compiles the public consumer fixtures
and rejects imports of removed legacy packages.

See the [Architecture](ARCHITECTURE.md), [Cookbook](examples/cookbook/README.md),
[Security Policy](SECURITY.md), and [Contributing Guide](CONTRIBUTING.md).

## License

Apache License 2.0. See [LICENSE](LICENSE), [NOTICE](NOTICE), and
[SOURCE_ATTRIBUTION.md](SOURCE_ATTRIBUTION.md).
