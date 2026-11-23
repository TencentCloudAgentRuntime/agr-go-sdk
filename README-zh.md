# 腾讯云 Agent Runtime Go SDK

[English](README.md)

这是腾讯云 Agent Runtime（AGR）的原生 Go SDK。`Client` 是凭证、地域、传输策略和
一级资源的产品入口。当前版本提供受管理的 `Sandbox` 生命周期和数据面操作、Deployment
控制面与 HTTP/SSE/WebSocket 连接、稳定的 Tool 管理能力，以及镜像预热任务。

这是一个新的 Go module，不是 `ags-go-sdk` 的原地升级版本。现有用户应先阅读
[从 ags-go-sdk 迁移](MIGRATING_FROM_AGS_GO_SDK-zh.md)。后续一级资源只有在公开服务契约和
用户旅程通过评审后才会加入；仓库不提供占位 API。

## 使用条件

- Go 1.22 或更高版本。
- 已开通腾讯云 Agent Sandbox。
- 目标地域中已有可用的 Sandbox Tool。
- 运行环境可以访问腾讯云 API 和沙箱数据面。

## 安装

```bash
go get github.com/TencentCloudAgentRuntime/agr-go-sdk
```

## 鉴权

控制面和 Metrics 使用腾讯云凭证：

```bash
export TENCENTCLOUD_REGION=ap-guangzhou
export TENCENTCLOUD_SECRET_ID=your-secret-id
export TENCENTCLOUD_SECRET_KEY=your-secret-key
# 使用腾讯云临时凭证时设置：
export TENCENTCLOUD_TOKEN=your-session-token
```

SDK 按需通过控制面取得沙箱实例和 Deployment 访问材料。该材料只在 SDK 内部使用。
公共 API 不接收或返回该材料，也不会把它写入 URL。

## 快速开始

单一云身份场景可以使用 `sandbox` 包。它读取并缓存上述环境配置：

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
	defer sb.Close() // 只释放本地资源

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

## 显式 Client 与多云身份

需要明确传入凭证、地域或传输配置时，使用 `agr.Client`。一个 Client 只绑定一个逻辑
云身份。同一进程使用多个云账号时，应为每套凭证分别创建 Client。

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

快捷入口和显式 Client 都返回 `*agr.Sandbox`，并调用同一套控制面、Metrics、生命周期
和数据面实现。

如果 Sandbox 使用自定义数据面域名后缀，在对应 Client 上设置
`agr.WithSandboxDomain("ap-guangzhou.example.com")`。传入完整主机名后缀，不带协议和端口。
此时 `sb.GetHost(8080)` 返回 `8080-<sandbox-id>.ap-guangzhou.example.com`，SDK 内部文件、
命令和 Code 连接也使用同一后缀。`WithControlPlaneEndpoint` 单独设置云 API 地址；
`WithDataPlaneDomain` 仍只影响 Deployment。自定义域名需要实际可用的 DNS、TLS 和路由；
`GetHost` 不探测 Tool 是否开放 8080 端口。参见
[显式 Client Cookbook](examples/cookbook/02_explicit_client/main.go)。

读取小型 UTF-8 文件可用 `sb.Files().ReadText(ctx, path, agr.ReadOptions{}, maxBytes)`：
SDK 会关闭流，并在超过显式大小上限时报错；`maxBytes` 必须为正且不超过 16 MiB。
大文件或二进制文件继续使用流式 `Read`。跨页遍历沙箱可用
`iterator := client.Sandboxes().Iterate(options)`，调用 `iterator.Next(ctx)` 直到 `io.EOF`，
并使用 `defer iterator.Close()`。迭代器最多保留一页，可取消，不保证变化中的远端列表具有
快照语义；需要自行控制游标时继续使用 `ListPage`。参见
[文件 Cookbook](examples/cookbook/01_default/main.go) 和
[列表 Cookbook](examples/cookbook/05_cursor_list/main.go)。

Tool 和预热任务是账号级控制面资源，只通过显式 Client 提供：

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

Deployment 也只通过显式 Client 使用。控制操作返回不可变快照；`Connect` 返回绑定一个
Tool 端口的本地连接：

```go
deployment, err := client.Deployments().WaitFor(ctx, deploymentID, agr.DeploymentActive)
if err != nil {
	return err
}
connection, err := client.Deployments().Connect(ctx, deployment.ID, agr.DeploymentConnectOptions{Port: 8080})
if err != nil {
	return err
}
defer connection.Close() // 只关闭本地 HTTP/SSE/WebSocket 操作

response, err := connection.Do(ctx, agr.DeploymentRequest{Method: http.MethodGet, Path: "/health"})
```

普通 HTTP 和流式读取 SSE 都使用 `Do`。`DialWebSocket` 返回 SDK 自有、支持 context 的
WebSocket handle。访问 Token、目标 authority 和 affinity 状态不进入公共 API，请求也不能
选择其他主机。完整流程见 [Deployment Cookbook](examples/cookbook/08_deployment/main.go)。

`Client` 的边界有意大于 Sandbox。后续一级资源只有在服务契约、生命周期、错误、测试和
Cookbook 用户旅程齐备后才会作为同级服务加入；仓库不发布占位服务。

## 能力

| 范围 | API |
| --- | --- |
| Sandbox 控制面 | `Create`、`Connect`、`Get`、偏移量 `List`、游标 `ListPage`/`Iterate`、`Pause`、`Resume`、`WaitFor`、`Update`、`Delete` |
| 文件 | 流式 `Read`/`Write`、有界 UTF-8 `ReadText`、`Stat`、`Exists`、`List`、`MakeDir`、`Move`、`Remove`、`Watch` |
| 命令 | `Run`、`Start`、`List`、`Connect`、有界输出、输入和信号 |
| Code | `Run`、受管理 Context、显式外部 Context 引用、有界回调 |
| PTY | 启动屏障、输入、窗口调整、有界事件流 |
| Metrics | 十项类型化指标，支持 `Start`、`End`、采样周期和部分成功 |
| Tool | `Create`、精确 `Get`、偏移量 `List`、`Update`、`WaitFor`、幂等 `Delete` |
| 镜像预热 | 按镜像、摘要和镜像仓库类型执行 `Create`、`Get` |
| Deployment 控制面 | `Create`、`Get`、`List`、`Update`、`WaitFor`、`Delete`、`WaitDeleted` |
| Deployment 数据面 | 直连受管 HTTP/SSE `Do`、支持 context 的 WebSocket、Token 刷新、affinity |

## 生命周期规则

- `Sandbox.Close()` 只释放本地 reader、流和观察任务，不删除远端实例。
- `Sandbox.Delete(ctx)` 显式停止远端实例，NotFound 按幂等成功处理。
- Pause 在提交控制面请求前使当前数据面 generation 失效。Resume 获取新的连接材料。旧
  reader、handle、watch、PTY、Code execution 和受管理 Context 不会恢复。
- context 取消只停止本地观察，不能证明远端 mutation 或进程已撤销。
- SDK 不自动重试 mutation。
- Tool mutation 回执只表示服务已受理，不是 SDK 拼装的 `ToolInfo` 快照。`ToolInfo` 和预热
  结果都是值对象，不需要 `Close`。
- 删除 Tool 不会级联删除 Sandbox。应先删除依赖实例，再删除本次流程创建的 Tool ID。
- 创建预热任务不表示镜像已就绪。应使用返回的完整镜像、摘要和仓库类型引用轮询 `Get`。
  稳定输入只支持 `enterprise` 和 `personal`；服务行为未验证前不承诺 `custom`。
- `DeploymentConnection.Close()` 取消本连接正在进行的 HTTP/SSE/WebSocket 操作，不删除
  Deployment。删除是异步操作，调用方应显式调用 `Delete` 和 `WaitDeleted`。
- Deployment Token 按需获取并由 SDK 内部刷新和脱敏，不作为公共参数或返回值。服务返回
  401 时，SDK 将响应交给调用方并使缓存 Token 失效，不自动重放请求。

Create 和 Resume 接受 30 秒至 24 小时的整秒 Timeout。Update 最低为 300 秒。Connect
先读取实例状态：PAUSED 使用 Resume 规则，RUNNING 使用 Update 规则。SDK 会原样发送
合法值；已部署的 Cloud 策略仍可能拒绝 30～299 秒，此时 SDK 返回原始结构化服务错误，
不会提升参数或自动重试。

## 错误处理

使用 `errors.As` 读取 `*agr.Error`。稳定字段包括 `Code`、`Reason`、`Operation`、
`RequestID`、`Retryable`、`InstanceID`、`ResourceType`、`ResourceID` 和可选 mutation
证据。格式化错误不会输出凭证、授权头、资源 ID、Cause 或响应正文。

## 开发

```bash
make verify
```

默认测试不会访问云端。真实云测试必须显式启用，具体变量和清理要求见
[贡献指南](CONTRIBUTING-zh.md)。协议与控制面来源记录在 [`contracts/`](contracts/) 中。
v2 控制面 manifest 固定 CLI revision 和来源 channel，记录有效契约哈希，以及各 Action
所属资源、成熟度、typed route、请求/响应类型和引用对象闭包。

存在 Preview 能力时，通过 `v0.3.0-preview.1` 一类语义化预发布版本交付。module path 和
公共资源名称保持不变。SDK 不提供运行时 Preview 开关、环境变量切换、请求级 Channel 或
Go build tag。上方能力表当前均为 stable；Preview 版本只有在契约、公共 API、测试和真实
服务证据通过评审后才会发布。

实现边界见 [架构说明](ARCHITECTURE-zh.md)，更多示例见
[Cookbook](examples/cookbook/README.md)。安全问题请按 [安全策略](SECURITY.md) 报告。

## 许可证

本项目使用 Apache License 2.0。详见 [LICENSE](LICENSE)、[NOTICE](NOTICE) 和
[SOURCE_ATTRIBUTION.md](SOURCE_ATTRIBUTION.md)。
