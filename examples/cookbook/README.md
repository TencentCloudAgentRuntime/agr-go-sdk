# Cookbook

Each example is a small, compilable user journey. Replace placeholder Tool or Sandbox IDs before
running it. All creation examples close local resources and delete only the instance they created.

| Journey | Example | Contract |
| --- | --- | --- |
| Environment-backed create, command, bounded text read, delete | [01_default](01_default/main.go) | `default-create-command-delete` |
| Explicit Client, Sandbox domain, and isolated configuration | [02_explicit_client](02_explicit_client/main.go) | `explicit-client-lifecycle`, `multiple-cloud-identities` |
| Managed Code context | [03_code_context](03_code_context/main.go) | `code-context` |
| Metrics with Start and End | [04_metrics](04_metrics/main.go) | `metrics-window` |
| Lazy opaque cursor iteration | [05_cursor_list](05_cursor_list/main.go) | `cursor-list` |
| Create Tool, run Sandbox, clean up | [06_tool_sandbox](06_tool_sandbox/main.go) | `tool-sandbox` |
| Create and observe image pre-cache | [07_pre_cache](07_pre_cache/main.go) | `pre-cache-image` |
| Deployment control and HTTP/SSE/WebSocket data plane | [08_deployment](08_deployment/main.go) | Create, connect, delete |

The journey IDs are machine-readable in [`contracts/journeys.json`](../../contracts/journeys.json).
File streaming, Watch, PTY, lifecycle, and cleanup behavior are covered by the root README and
offline contract tests.

The explicit Client example accepts `AGR_SANDBOX_DOMAIN` as a complete Sandbox hostname suffix.
Set `AGR_SANDBOX_ID` to connect to an existing Sandbox and print its port 8080 host. A custom
suffix must already have working DNS, TLS, and service routing. The example does not own or delete
that existing Sandbox.

The Tool example creates billable remote resources. It deletes the Sandbox before deleting only
the Tool identity returned by its own Create call. The pre-cache example expects
`AGR_PRECACHE_IMAGE` to name an image readable by the current account and
`AGR_PRECACHE_REGISTRY_TYPE` to be `enterprise` or `personal`. The stable SDK contract does not
claim support for the service's unverified `custom` registry value.

The Deployment example expects `AGR_TOOL_ID` and `AGR_DEPLOYMENT_PORT`. The referenced Tool must
serve the example's `/health`, `/events`, and `/ws` routes on that port. The SDK obtains and
refreshes the Deployment access token internally. Closing the connection stops local HTTP/SSE/
WebSocket work; only `Deployments().Delete` removes the remote resource.
