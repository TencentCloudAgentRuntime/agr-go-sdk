# Cookbook

Each example is a small, compilable user journey. Replace placeholder Tool or Sandbox IDs before
running it. All creation examples close local resources and delete only the instance they created.

| Journey | Example | Contract |
| --- | --- | --- |
| Environment-backed create, command, delete | [01_default](01_default/main.go) | `default-create-command-delete` |
| Explicit Client and isolated configuration | [02_explicit_client](02_explicit_client/main.go) | `explicit-client-lifecycle`, `multiple-cloud-identities` |
| Managed Code context | [03_code_context](03_code_context/main.go) | `code-context` |
| Metrics with Start and End | [04_metrics](04_metrics/main.go) | `metrics-window` |
| Opaque cursor pagination | [05_cursor_list](05_cursor_list/main.go) | `cursor-list` |
| Create Tool, run Sandbox, clean up | [06_tool_sandbox](06_tool_sandbox/main.go) | `tool-sandbox` |
| Create and observe image pre-cache | [07_pre_cache](07_pre_cache/main.go) | `pre-cache-image` |
| Deployment control and HTTP/SSE/WebSocket data plane | [08_deployment](08_deployment/main.go) | Create, connect, delete |
| Preview persistent Volume mounts | [09_persistent_volume](09_persistent_volume/main.go) | Volume, VolumeTemplate, Tool mount, Sandbox cleanup |

The journey IDs are machine-readable in [`contracts/journeys.json`](../../contracts/journeys.json).
File streaming, Watch, PTY, lifecycle, and cleanup behavior are covered by the root README and
offline contract tests.

The Tool example creates billable remote resources. It deletes the Sandbox before deleting only
the Tool identity returned by its own Create call. The pre-cache example expects
`AGR_PRECACHE_IMAGE` to name an image readable by the current account and
`AGR_PRECACHE_REGISTRY_TYPE` to be `enterprise` or `personal`. The stable SDK contract does not
claim support for the service's unverified `custom` registry value.

The Deployment example expects `AGR_TOOL_ID` and `AGR_DEPLOYMENT_PORT`. The referenced Tool must
serve the example's `/health`, `/events`, and `/ws` routes on that port. The SDK obtains and
refreshes the Deployment access token internally. Closing the connection stops local HTTP/SSE/
WebSocket work; only `Deployments().Delete` removes the remote resource.

The persistent-volume example is available only from a Preview release. It requires a reviewed,
disposable CFS fixture through `AGR_VOLUME_CFS_FILESYSTEM_ID`, an envd-capable
`AGR_VOLUME_TOOL_TYPE`, and any role ARNs required by the account. It never deletes the CFS file
system. A temporary bootstrap Tool/Sandbox creates and later removes only the example's unique
CFS subpath. Cleanup runs in dependency order: workload Sandbox, Tool, retained derived Volumes,
VolumeTemplate, standalone Volume, fixture subpath, bootstrap Sandbox, then bootstrap Tool.
