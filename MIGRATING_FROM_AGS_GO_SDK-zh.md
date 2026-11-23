# 从 `ags-go-sdk` 迁移

[English](MIGRATING_FROM_AGS_GO_SDK.md)

本文以已发布的 `ags-go-sdk` **v0.1.5** 为旧版基线。该版本与旧仓库 `main`
同为提交 `351c2e5`。旧仓库未发布的 `codex/sdk-v0-convergence` 分支已经改变了
公共 API，不属于本文的对比范围。生产项目迁移时，请固定已发布的 `agr-go-sdk`
版本。

`agr-go-sdk` 是产品级的新 Go module，从完整的托管 Sandbox 旅程起步，并通过
`Client` 承载其他经过评审的 AGR 一级资源。它不是 `ags-go-sdk` 的源码兼容版本。

建议按应用或组件逐个迁移。不要混用两个 module 的句柄、生成消息、凭证或数据面
Client。

## Module 与包路径

```diff
- github.com/TencentCloudAgentRuntime/ags-go-sdk
+ github.com/TencentCloudAgentRuntime/agr-go-sdk
```

根包名称由 `ags` 改为 `agr`：

```go
import agr "github.com/TencentCloudAgentRuntime/agr-go-sdk"
```

在现有消费者完成迁移前，旧 module 继续作为独立维护线存在。本仓库不会重建其深层包
作为兼容层。

## 破坏性变更一览

| v0.1.5 用法 | AGR 替代方式或影响 |
| --- | --- |
| `sandbox/core`、`sandbox/code`、`tool/*`、`connection`、`constant`、`pb/*` 导入 | 改用根包 `agr` 和 `sandbox` 快捷入口。深层包与公开生成类型没有源码兼容替代包。 |
| `Create(ctx, toolName, options...)` | 传入 `CreateOptions{Tool: ToolRef{Name: toolName}}`；精确指定 Tool ID 时使用 `ToolRef{ID: id}`。选项改为值类型字段，不再使用旧版单次调用 option 函数。 |
| `WithClient(*ags.Client)` 或 `common.CredentialIface` | 使用 `WithCredential`、`WithTemporaryCredential` 或 `WithCredentialProvider` 构造 `agr.Client`。不再支持注入生成的 Cloud Client。 |
| 用于 Sandbox 的 `WithDataPlaneDomain("example.com")` | 改用 `WithSandboxDomain("ap-guangzhou.example.com")`：新值包含地域，同时作用于 Sandbox 端口 Host 和托管数据面连接。`WithDataPlaneDomain` 现**仅用于 Deployment**。 |
| `WithScheme("http")`、可变 `ConnectionConfig` 或携带实例 Token 调用 `tool/*`.New | 不提供仅凭实例 Token 的数据面直连入口，也不提供 HTTP Scheme 开关。托管连接通过云凭证获取访问材料并使用 HTTPS。依赖直连 Token 访问的调用方不能原样迁移该路径。 |
| `sb.GetHost(port) string` | `sb.GetHost(port) (string, error)`，端口不合法时返回错误。 |
| `sb.Kill(ctx)` 或包级 `Kill` | 改用 `sb.Delete(ctx)` 或 `client.Sandboxes().Delete(ctx, id)`。`Close()` 只释放本地句柄。 |
| `sb.Files`、`sb.Commands`、`sb.Code` 字段 | 改调 `sb.Files()`、`sb.Commands()`、`sb.Code()`；不要在 Sandbox 关闭后继续持有这些服务对象。 |
| `List(ctx)` 返回 `[]*ags.SandboxInstance` | `List(ctx, SandboxListOptions{})` 返回 SDK 自有值类型的单页 `SandboxPage`；游标跨页遍历使用 `ListPage` 或 `Iterate`。 |
| `RunCodeConfig.ContextId` | 改用受管理的 `CodeContext` 或显式 `NewExternalCodeContextRef(id)`；后者依赖服务端校验。 |

旧版 SDK 允许通过 `connection.Config{Domain, AccessToken, Headers, Proxy, Scheme}`
直接构造 Files、Commands 和 Code Client。新版刻意不提供任意实例 Token、自定义数据面
Header 或手工指定数据面 URL 的对应入口。`WithHTTPClient` 可传入受信任的 HTTP
传输配置，但不会恢复上述公开连接控制项。请在修改导入前先盘点这类用法。

## 入口对照

| 旧版 API | AGR API | 说明 |
| --- | --- | --- |
| `sandbox/core.Create`、`sandbox/code.Create` | `sandbox.Create` 或 `client.Sandboxes().Create` | 两种入口均返回 `*agr.Sandbox`。 |
| `sandbox/core.Connect`、`sandbox/code.Connect` | `sandbox.Connect` 或 `client.Sandboxes().Connect` | Connect 可能恢复暂停实例或延长寿命，并验证数据面就绪。 |
| 包级 `List` | `sandbox.List` 或 `client.Sandboxes().List` | 返回 SDK 自有的 `SandboxPage`。 |
| 包级 `Kill` | `Sandbox.Delete` 或 `client.Sandboxes().Delete` | Delete 为显式远端删除，NotFound 视为幂等成功。 |
| `WithClient(*generated.Client)` | `agr.NewClient(...)` | 生成的 Cloud Client 不再作为公共配置项。 |
| 生成的凭证接口 | `agr.CredentialProvider` | 一个 Client 绑定一个逻辑云身份。 |

默认快捷入口读取 `TENCENTCLOUD_REGION`、`TENCENTCLOUD_SECRET_ID`、
`TENCENTCLOUD_SECRET_KEY` 和可选的 `TENCENTCLOUD_TOKEN`。多身份场景应创建独立的
显式 Client。

对于 v0.1.5 常见的 `sandbox/code.Create(ctx, toolName, ...)` 路径，新版创建方式如下：

```go
func run(ctx context.Context, toolName string) (err error) {
	lifetime := 5 * time.Minute // 保留旧版 SDK 的默认 5 分钟超时。
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
	// 在此调用 sb.Files()、sb.Commands() 或 sb.Code()。
	return nil
}
```

旧版 SDK 创建沙箱默认请求 5 分钟；AGR 中 `Timeout == nil` 表示交由服务端选择默认值。
旧版字符串形式的 `SandboxConfig.Timeout` 和指针挂载字段改为 `*time.Duration` 和
`[]agr.MountOption`；`AuthMode` 使用 `agr.SandboxAuthMode` 常量。完整清理写法见
[默认创建与清理 Cookbook](examples/cookbook/01_default/main.go)。

## Sandbox 与服务

| 旧版 API | AGR API |
| --- | --- |
| `SandboxId` | `ID()` |
| `GetInfo` | `Info(ctx)` |
| `SetTimeoutSeconds` | `Update(ctx, UpdateOptions{Timeout: ...})` |
| `Kill` | `Delete(ctx)` |
| `sb.Files`、`sb.Commands`、`sb.Code` | `sb.Files()`、`sb.Commands()`、`sb.Code()` |
| 可变连接配置 | 由 Create、Connect 或 Resume 获取的内部 generation |
| `constant.EnvdPort`、`constant.CodePort` | 无公开替代常量 |
| `constant.AgentSandboxInternalEndpoint` | 经批准的端点覆盖使用 `WithControlPlaneEndpoint` |

`Connect` 行为也有变化：v0.1.5 获取实例 Token 后直接返回，不探测数据面。AGR 会先读取
实例状态，可能执行 Resume 或续期，并等待数据面就绪。如果只需读取元数据，请改用
`client.Sandboxes().Get(ctx, id)`，不要仅为了展示实例信息而调用 `Connect`。

旧版 Sandbox 域名选项会自动在基础域名前拼接云地域。AGR 要求传入完整后缀：

```go
client, err := agr.NewClient(
	agr.WithRegion("ap-guangzhou"),
	agr.WithSandboxDomain("ap-guangzhou.example.com"),
)
// 后续：host, err := sb.GetHost(8080)
```

`WithControlPlaneEndpoint` 与该 Sandbox 域名相互独立。自定义域名需要服务端具备可用的
DNS、TLS 和路由；`GetHost` 只计算主机名，不检查端口可达性。完整示例见
[显式 Client 与域名 Cookbook](examples/cookbook/02_explicit_client/main.go)。

`Close()` 仅释放本地资源。对于应用拥有的远端实例，应显式调用 `Delete`，并使用独立的
清理 context，避免业务 context 已取消导致清理失败。

创建使用 `agr.CreateOptions`、`agr.ToolRef`、`agr.MountOption` 和 `agr.SandboxAuthMode`。
List、Get、Metrics、Files、Commands、PTY、Watch 和 Code 均使用 SDK 自有值类型。
生成的腾讯云 SDK、protobuf 和 Connect 类型属于私有实现细节。

超时统一使用 `*time.Duration`。Create 和 Resume 接受 30 秒至 24 小时的整秒值；Update
接受 300 秒至 24 小时的整秒值。SDK 不会静默截断或抬高参数；若已部署的云端策略拒绝，
SDK 会返回原始结构化 `*agr.Error`。

## 文件、命令与 PTY

- `Read` 返回 `io.ReadCloser`；完整读取和提前退出都必须调用 `Close()`。
- `Write` 接受 `io.Reader`，流式上传而不在内存中缓存整份文件。
- `Rename` 改为 `Move`。`MakeDir`、`Move` 和 `Write` 返回 `FileInfo` 值对象，替代旧版的
  bool、仅 error 和 `WriteInfo`。`GetInfo` 改为 `Stat`；文件信息保留 owner、group、mode、
  permissions 和可选符号链接目标。请检查依赖旧版文件类型字符串的代码。
- 旧版命令在 `ProcessConfig.Args` 为空时会自动通过 `/bin/bash -l -c` 执行 `"echo hello"`。
  AGR 直接传递可执行文件与参数。如需保留旧版 Shell 行为，请显式调用
  `sb.Commands().Run(ctx, "/bin/bash", agr.CommandOptions{Args: []string{"-lc", "echo hello"}})`。
- 前台命令的 stdout 和 stderr 各自独立设限，并在结果中报告是否截断。
- `Run` 接收值类型选项并返回 `CommandResult` 值对象。不再提供 `OnOutputConfig` 参数；
  实时输出请使用 `Start` 并读取 `Events()` 通道。非零退出码记录在结果中，而不是作为
  Go 传输层错误返回。
- `Start` 和 `Connect` 返回绑定当前 generation 的 `CommandHandle`。使用 `PID()` 替代旧版
  `Pid` 字段，使用 `Write(ctx, data)` 替代 `SendInput(ctx, pid, data)`。
- `CommandHandle.Close` 替代仅停止本地观察的 `Disconnect`；远程终止进程请使用
  `Signal(ctx, agr.SignalKILL)` 替代 `Kill`。关闭句柄不会杀死远端进程。
- PTY 返回有界事件流，且在通过启动屏障后才可交互。

## Code 上下文

`RunCodeConfig.ContextId` 改为 `RunCodeOptions.Context`：

```go
managed, err := sb.Code().CreateContext(ctx, agr.CreateCodeContextOptions{})
result, err := sb.Code().Run(
	ctx,
	source,
	agr.RunCodeOptions{Context: managed},
	callbacks,
)
```

受管理的 `CodeContext` 绑定单个 Sandbox generation。跨 Sandbox 使用或在 Pause/Resume 后
复用会在本地直接报错。如需显式交由服务端校验已有上下文 ID：

```go
external, err := agr.NewExternalCodeContextRef(existingContextID)
result, err := sb.Code().Run(
	ctx,
	source,
	agr.RunCodeOptions{Context: external},
	callbacks,
)
```

外部引用不保证该上下文存在、归属当前实例或在生命周期切换后仍然有效；SDK 也不会自动
新建替代上下文来伪装恢复。完整示例见 [Code 上下文 Cookbook](examples/cookbook/03_code_context/main.go)。

`RunCode` 改为 `sb.Code().Run(ctx, source, options, callbacks)`。旧版的
`Execution.Logs.Stdout/Stderr` 移至 `CodeExecution.Stdout/Stderr`；富文本结果使用带 Go
首字母缩写的 `CodeResult`（如 `HTML`、`JSON`、`PNG`）。保留输出和回调均有大小上限。
若所选 Tool 未提供 Code 服务，`Code().Run` 仍需要合适的 Tool，仅创建 Sandbox 不会自动
附加该服务。

## 错误处理

请将字符串匹配替换为 `errors.As(err, &sdkError)`，并按 `sdkError.Code` 和
`sdkError.Reason` 分支处理。服务端受理后的 Create 失败可能携带 `InstanceID`；mutation
结果不明确时不要直接重试。

## 消费者迁移门禁

在移除 `ags-go-sdk` 依赖前，请确认：

1. 盘点所有直连实例 Token Client、自定义 Scheme/Header/Proxy 以及直接使用 protobuf 或
   腾讯云生成模型的位置，在移除旧包前先确定替代接入方式；
2. 替换旧导入（包括深层 `connection`、`constant`、`pb`、`sandbox/*` 和 `tool/*` 路径），
   并固定已发布的 AGR 版本；
3. 使用消费者自身声明的 Go 工具链完成编译，并运行其确定性单测及 AGR SDK 消费者夹具；
4. 验证命令的 Shell 行为、输出上限、Reader 与句柄显式关闭、`Connect` 副作用、超时默认值
   以及显式远端删除；
5. 涉及生命周期或数据面变更时，执行独立授权的真实云端 E2E 验证，确认清理干净且日志中
   不含凭证或实例访问材料。

`ags-cli` 当前仍导入旧版深层包；它的迁移属于独立的下游改动，不随本 module 创建自动完成。
